package dbgate

import (
	"bytes"
	"context"
	"database/sql/driver"
	"fmt"
	"net"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// sshParam is the connection string query parameter naming a host alias from the human's own
// ssh config. The database is then dialed through the system ssh binary, so ProxyJump, ssh-agent,
// known_hosts and ControlMaster behave as they do in the human's terminal.
const sshParam = "lgrass_ssh"

// sshBinary is a variable so tests can stand in a fake ssh.
var sshBinary = "ssh"

const (
	sshStderrCap = 4096
	// sshExitWait bounds how long a failed connect waits for ssh to exit before reporting the
	// driver's own error instead.
	sshExitWait = 2 * time.Second
	// sshWaitDelay stops Wait from blocking on a stderr pipe held open by a process ssh left
	// behind, such as a ControlPersist master.
	sshWaitDelay = time.Second
)

// The alias reaches ssh as an argument, so one starting with "-" would be read as an option such
// as -oProxyCommand=, which runs a command.
var sshAliasPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*$`)

// splitSSHAlias removes lgrass_ssh from connString and returns the remaining string and the
// alias, or connString unchanged and "" when it names none. A string that does not parse is
// returned as is, so the driver path reports it without echoing the credential.
func splitSSHAlias(connString string) (string, string, error) {
	u, err := url.Parse(connString)
	if err != nil {
		return connString, "", nil
	}
	q := u.Query()
	values, ok := q[sshParam]
	if !ok {
		return connString, "", nil
	}
	if len(values) != 1 || !sshAliasPattern.MatchString(values[0]) {
		return "", "", fmt.Errorf("dbgate: invalid ssh host alias %q", strings.Join(values, ","))
	}
	q.Del(sshParam)
	u.RawQuery = q.Encode()
	return u.String(), values[0], nil
}

// sshDialFunc returns a driver dial function that reaches addr through alias and records each
// conn it makes on the context sshConnector passes, so a failed connect can report ssh's error.
func sshDialFunc(alias string) func(ctx context.Context, network, addr string) (net.Conn, error) {
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		conn, err := dialSSH(alias, network, addr)
		if err != nil {
			return nil, err
		}
		if dials, ok := ctx.Value(sshDialsKey{}).(*sshDials); ok {
			dials.add(conn)
		}
		return conn, nil
	}
}

// dialSSH runs `ssh -W addr alias` with one end of a socketpair as its stdin and stdout and
// returns the other end. A socket rather than pipes gives the driver read and write deadlines,
// which pgx relies on to cancel a query.
func dialSSH(alias, network, addr string) (*sshConn, error) {
	switch network {
	case "tcp", "tcp4", "tcp6":
	default:
		return nil, fmt.Errorf("dbgate: ssh carries only tcp connections, not %s", network)
	}
	local, remote, err := socketPair()
	if err != nil {
		return nil, err
	}
	defer remote.Close()

	stderr := &cappedBuffer{max: sshStderrCap}
	cmd := exec.Command(sshBinary, "-o", "BatchMode=yes", "-W", addr, "--", alias)
	cmd.Stdin = remote
	cmd.Stdout = remote
	cmd.Stderr = stderr
	cmd.WaitDelay = sshWaitDelay
	if err := cmd.Start(); err != nil {
		local.Close()
		return nil, fmt.Errorf("dbgate: starting ssh: %w", err)
	}

	c := &sshConn{Conn: local, cmd: cmd, stderr: stderr, done: make(chan struct{})}
	go func() {
		_ = cmd.Wait()
		close(c.done)
	}()
	return c, nil
}

// sshConn is the driver's side of the socketpair, tied to the ssh process carrying it.
type sshConn struct {
	net.Conn
	cmd       *exec.Cmd
	stderr    *cappedBuffer
	done      chan struct{}
	closeOnce sync.Once
}

func (c *sshConn) Close() error {
	err := c.Conn.Close()
	c.closeOnce.Do(func() { _ = c.cmd.Process.Kill() })
	return err
}

// failure returns ssh's own error once it has exited with a non-zero code. A process killed by
// Close is no failure: the driver closed the conn after an error of its own.
func (c *sshConn) failure() error {
	select {
	case <-c.done:
	case <-time.After(sshExitWait):
		return nil
	}
	state := c.cmd.ProcessState
	if state == nil || !state.Exited() || state.ExitCode() == 0 {
		return nil
	}
	msg := strings.Join(strings.Fields(strings.ReplaceAll(c.stderr.String(), "\r", "")), " ")
	if msg == "" {
		msg = fmt.Sprintf("exited with code %d", state.ExitCode())
	}
	return fmt.Errorf("dbgate: ssh: %s", msg)
}

type sshDialsKey struct{}

type sshDials struct {
	mu    sync.Mutex
	conns []*sshConn
}

func (d *sshDials) add(c *sshConn) {
	d.mu.Lock()
	d.conns = append(d.conns, c)
	d.mu.Unlock()
}

func (d *sshDials) failure() error {
	d.mu.Lock()
	conns := append([]*sshConn(nil), d.conns...)
	d.mu.Unlock()
	for _, c := range conns {
		if err := c.failure(); err != nil {
			return err
		}
	}
	return nil
}

// sshConnector reports ssh's error in place of the driver's when a connect fails because the
// tunnel did, since a driver sees only a closed socket and go-sql-driver/mysql turns that into
// "invalid connection".
type sshConnector struct {
	driver.Connector
}

func (c sshConnector) Connect(ctx context.Context) (driver.Conn, error) {
	dials := &sshDials{}
	conn, err := c.Connector.Connect(context.WithValue(ctx, sshDialsKey{}, dials))
	if err != nil {
		if sshErr := dials.failure(); sshErr != nil {
			return nil, sshErr
		}
	}
	return conn, err
}

// cappedBuffer keeps the first max bytes written to it. os/exec copies stderr from its own
// goroutine, hence the lock.
type cappedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
	max int
}

func (b *cappedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if room := b.max - b.buf.Len(); room > 0 {
		b.buf.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

func (b *cappedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
