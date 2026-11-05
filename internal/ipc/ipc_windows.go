// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package ipc

import (
	"bufio"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"

	"github.com/faizalv/lemongrass/internal/atomicfile"
	"github.com/faizalv/lemongrass/internal/config"
	"github.com/faizalv/lemongrass/internal/privfs"
)

const pipePrefix = `\\.\pipe\`

// greetingMagic starts the line a Windows server writes first on every connection. No reply of any lemongrass
// protocol starts with a NUL byte, so a Go client can tell the greeting from a reply that a Node server sends.
const greetingMagic = "\x00lgipc "

// Address is the named pipe for name. The SID keeps users apart, though any user can still create any pipe name:
// Dial checks the server's owner, and Node clients check the greeting token instead.
func Address(name string) string {
	sid, err := privfs.UserSID()
	if err != nil {
		sid = "unknown"
	}
	return pipePrefix + "lemongrass-" + sid + "-" + name
}

// pipeName maps a filesystem path, as tests and the flutter supervisor pass, onto a pipe of its own.
func pipeName(addr string) string {
	if strings.HasPrefix(strings.ToLower(addr), pipePrefix) {
		return addr
	}
	if abs, err := filepath.Abs(addr); err == nil {
		addr = abs
	}
	sum := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(addr))))
	return Address("p" + hex.EncodeToString(sum[:])[:16])
}

// TokenPath is the owner-only file holding the greeting token of the server at addr. A pipe from Address keeps it
// under ~/.lemongrass, where ipcAddress.ts reads it; a path address keeps it beside the path.
func TokenPath(addr string) string {
	own := Address("")
	if strings.HasPrefix(strings.ToLower(addr), strings.ToLower(own)) {
		return filepath.Join(config.Dir(), addr[len(own):]+".ipckey")
	}
	if strings.HasPrefix(strings.ToLower(addr), pipePrefix) {
		return ""
	}
	return addr + ".ipckey"
}

// pipeBufferSize is what libuv gives Node's pipes. With go-winio's default of zero a write completes only once the
// other side reads it, so the greeting and the client's first request, both written before either side reads, would
// block each other.
const pipeBufferSize = 64 << 10

// The DACL lets only the current user open the pipe. go-winio creates the first instance with FILE_CREATE, so a name
// already taken, by a running daemon or by a squatter, fails here instead of sharing the name, and it rejects remote
// clients.
func listen(addr string) (net.Listener, error) {
	sid, err := privfs.UserSID()
	if err != nil {
		return nil, err
	}
	name := pipeName(addr)
	l, err := winio.ListenPipe(name, &winio.PipeConfig{
		SecurityDescriptor: "D:P(A;;GA;;;" + sid + ")",
		InputBufferSize:    pipeBufferSize,
		OutputBufferSize:   pipeBufferSize,
	})
	if err != nil {
		return nil, fmt.Errorf("ipc: listening on %s: %w", name, err)
	}
	greeting, err := writeToken(TokenPath(addr))
	if err != nil {
		l.Close()
		return nil, err
	}
	return &greetListener{Listener: l, greeting: greeting}, nil
}

// writeToken stores a fresh token after the pipe exists, so a second daemon that lost the name never overwrites the
// token of the one that holds it.
func writeToken(path string) ([]byte, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return nil, fmt.Errorf("ipc: generating the pipe token: %w", err)
	}
	token := hex.EncodeToString(raw)
	if path != "" {
		if err := privfs.MkdirAll(filepath.Dir(path)); err != nil {
			return nil, fmt.Errorf("ipc: %w", err)
		}
		if err := atomicfile.Write(path, []byte(token+"\n"), 0o600); err != nil {
			return nil, fmt.Errorf("ipc: writing the pipe token: %w", err)
		}
	}
	return []byte(greetingMagic + token + "\n"), nil
}

const greetingWriteTimeout = 2 * time.Second

// greetListener speaks first on every connection with the token, which only the current user can read from disk.
// That is how the app, whose Node client cannot ask who owns a pipe, tells our daemon from a pipe another user
// created first under the same name.
type greetListener struct {
	net.Listener
	greeting []byte
}

func (l *greetListener) Accept() (net.Conn, error) {
	for {
		conn, err := l.Listener.Accept()
		if err != nil {
			return nil, err
		}
		conn.SetWriteDeadline(time.Now().Add(greetingWriteTimeout))
		_, err = conn.Write(l.greeting)
		conn.SetWriteDeadline(time.Time{})
		if err != nil {
			conn.Close()
			continue
		}
		return conn, nil
	}
}

func dial(addr string, timeout time.Duration) (net.Conn, error) {
	name := pipeName(addr)
	conn, err := winio.DialPipe(name, &timeout)
	if err != nil {
		return nil, err
	}
	if err := verifyServer(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return &greetedConn{Conn: conn}, nil
}

// verifyServer refuses a pipe whose server process does not run as the current user.
func verifyServer(conn net.Conn) error {
	h, err := pipeHandle(conn)
	if err != nil {
		return err
	}
	var pid uint32
	if err := windows.GetNamedPipeServerProcessId(h, &pid); err != nil {
		return fmt.Errorf("ipc: reading the pipe server: %w", err)
	}
	same, err := sameUser(pid)
	if err != nil {
		return err
	}
	if !same {
		return errors.New("ipc: pipe server runs as another user")
	}
	return nil
}

// greetedConn drops the greeting line a Go server sends first; a Node server sends none, so the first byte decides.
type greetedConn struct {
	net.Conn
	once   sync.Once
	reader *bufio.Reader
	err    error
}

func (c *greetedConn) Read(p []byte) (int, error) {
	c.once.Do(func() {
		c.reader = bufio.NewReader(c.Conn)
		first, err := c.reader.Peek(1)
		if err != nil || first[0] != greetingMagic[0] {
			return
		}
		if _, err := c.reader.ReadSlice('\n'); err != nil {
			c.err = err
		}
	})
	if c.err != nil {
		return 0, c.err
	}
	return c.reader.Read(p)
}
