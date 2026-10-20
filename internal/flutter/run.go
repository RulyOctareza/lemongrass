package flutter

import (
	"bufio"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Run states written to run.json.
const (
	StatusBuilding = "building"
	StatusStarted  = "started"
	StatusStopped  = "stopped"
	StatusFailed   = "failed"
)

const restartTimeout = 3 * time.Minute

// RunState is what a running lgrass flutter run supervisor reports about its app.
type RunState struct {
	PID       int       `json:"pid"`
	Serial    string    `json:"serial"`
	Project   string    `json:"project"`
	Args      []string  `json:"args"`
	Status    string    `json:"status"`
	AppID     string    `json:"app_id,omitempty"`
	VMService string    `json:"vm_service,omitempty"`
	Message   string    `json:"message,omitempty"`
	StartedAt time.Time `json:"started_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func statePath(dir string) string { return filepath.Join(dir, "run.json") }

// LogPath is the full output of the flutter tool for the current run.
func LogPath(dir string) string { return filepath.Join(dir, "run.log") }

// SocketPath is the supervisor's control socket. It lives in the temp dir under a short name
// because a unix socket path is capped near 104 bytes on macOS.
func SocketPath(project string) string {
	sum := sha1.Sum([]byte(project))
	return filepath.Join(os.TempDir(), "lgf-"+hex.EncodeToString(sum[:])[:10]+".sock")
}

// ReadState returns the last run.json, or an error wrapping os.ErrNotExist when there was no run.
func ReadState(dir string) (RunState, error) {
	var s RunState
	data, err := os.ReadFile(statePath(dir))
	if err != nil {
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("flutter: reading run state: %w", err)
	}
	return s, nil
}

func writeState(dir string, s RunState) {
	s.UpdatedAt = time.Now()
	data, _ := json.MarshalIndent(s, "", "  ")
	tmp := statePath(dir) + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, statePath(dir))
	}
}

// Alive reports whether the supervisor process of a state still runs.
func Alive(s RunState) bool {
	if s.PID <= 0 || (s.Status != StatusBuilding && s.Status != StatusStarted) {
		return false
	}
	return syscall.Kill(s.PID, 0) == nil
}

// Spawn starts the supervisor detached from the calling shell, so the app keeps running and
// stays reloadable after the lgrass call returns. self is the lgrass binary.
func Spawn(self, project, serial string, args []string) error {
	dir, err := StateDir(project)
	if err != nil {
		return err
	}
	if s, err := ReadState(dir); err == nil && Alive(s) {
		return fmt.Errorf("flutter: an app is already running on %s from this project (status %s). Use lgrass flutter reload, restart or stop", s.Serial, s.Status)
	}
	log, err := os.Create(LogPath(dir))
	if err != nil {
		return fmt.Errorf("flutter: %w", err)
	}
	defer log.Close()
	writeState(dir, RunState{Serial: serial, Project: project, Args: args, Status: StatusBuilding, StartedAt: time.Now()})
	cmd := exec.Command(self, append([]string{"flutter", "_serve", project, serial, "--"}, args...)...)
	cmd.Dir = project
	cmd.Stdout = log
	cmd.Stderr = log
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("flutter: starting the run supervisor: %w", err)
	}
	return cmd.Process.Release()
}

// Wait polls run.json until the app has started or the run ended, up to timeout.
func Wait(project string, timeout time.Duration) (RunState, error) {
	dir, err := StateDir(project)
	if err != nil {
		return RunState{}, err
	}
	deadline := time.Now().Add(timeout)
	for {
		s, err := ReadState(dir)
		if err == nil && s.Status != StatusBuilding {
			return s, nil
		}
		if err == nil && s.PID > 0 && syscall.Kill(s.PID, 0) != nil {
			s.Status = StatusFailed
			s.Message = "the run supervisor exited"
			return s, nil
		}
		if time.Now().After(deadline) {
			return s, nil
		}
		time.Sleep(500 * time.Millisecond)
	}
}

// Control sends reload, restart or stop to the supervisor and returns its answer.
func Control(project, command string) (string, error) {
	conn, err := net.DialTimeout("unix", SocketPath(project), 2*time.Second)
	if err != nil {
		return "", errors.New("flutter: no app is running from this project. Start one with lgrass flutter run")
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(restartTimeout + 10*time.Second))
	if _, err := fmt.Fprintln(conn, command); err != nil {
		return "", fmt.Errorf("flutter: %w", err)
	}
	var reply struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(conn).Decode(&reply); err != nil {
		return "", fmt.Errorf("flutter: reading the supervisor's answer: %w", err)
	}
	if !reply.OK {
		return "", errors.New("flutter: " + reply.Message)
	}
	return reply.Message, nil
}

// supervisor owns one flutter run --machine process.
type supervisor struct {
	dir    string
	state  RunState
	stdin  io.Writer
	mu     sync.Mutex
	nextID int
	calls  map[int]chan machineResponse
	// onStarted runs once the app's VM service is reachable, and again after each restart.
	onStarted func(vmService string)
}

type machineResponse struct {
	ID     int             `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  json.RawMessage `json:"error"`
}

type machineEvent struct {
	Event  string          `json:"event"`
	Params json.RawMessage `json:"params"`
}

// Serve is the supervisor's body: it runs flutter run --machine and answers the control socket
// until the app stops. onStarted may be nil.
func Serve(project, serial string, args []string, onStarted func(vmService string)) error {
	dir, err := StateDir(project)
	if err != nil {
		return err
	}
	sdk, err := SDK(project)
	if err != nil {
		return err
	}
	sv := &supervisor{
		dir:       dir,
		state:     RunState{PID: os.Getpid(), Serial: serial, Project: project, Args: args, Status: StatusBuilding, StartedAt: time.Now()},
		calls:     map[int]chan machineResponse{},
		onStarted: onStarted,
	}
	writeState(dir, sv.state)

	cmd := exec.Command(sdk, append([]string{"run", "--machine", "-d", serial}, args...)...)
	cmd.Dir = project
	cmd.Stderr = os.Stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	sv.stdin = stdin
	if err := cmd.Start(); err != nil {
		sv.fail(err.Error())
		return err
	}

	sock := SocketPath(project)
	_ = os.Remove(sock)
	listener, err := net.Listen("unix", sock)
	if err != nil {
		_ = cmd.Process.Kill()
		sv.fail("control socket: " + err.Error())
		return err
	}
	defer os.Remove(sock)
	go sv.accept(listener)

	scanner := bufio.NewScanner(stdout)
	scanner.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		fmt.Println(line)
		sv.handleLine(line)
	}
	err = cmd.Wait()
	listener.Close()
	sv.mu.Lock()
	defer sv.mu.Unlock()
	if sv.state.Status == StatusStarted || sv.state.Status == StatusStopped {
		sv.state.Status = StatusStopped
		if sv.state.Message == "" {
			sv.state.Message = "the app stopped"
		}
	} else {
		sv.state.Status = StatusFailed
		if sv.state.Message == "" && err != nil {
			sv.state.Message = err.Error()
		}
	}
	writeState(dir, sv.state)
	return nil
}

func (sv *supervisor) fail(message string) {
	sv.mu.Lock()
	defer sv.mu.Unlock()
	sv.state.Status = StatusFailed
	sv.state.Message = message
	writeState(sv.dir, sv.state)
}

func (sv *supervisor) handleLine(line string) {
	if !strings.HasPrefix(line, "[{") {
		return
	}
	var batch []json.RawMessage
	if json.Unmarshal([]byte(line), &batch) != nil {
		return
	}
	for _, raw := range batch {
		var ev machineEvent
		if json.Unmarshal(raw, &ev) == nil && ev.Event != "" {
			sv.handleEvent(ev)
			continue
		}
		var resp machineResponse
		if json.Unmarshal(raw, &resp) == nil {
			sv.mu.Lock()
			ch := sv.calls[resp.ID]
			delete(sv.calls, resp.ID)
			sv.mu.Unlock()
			if ch != nil {
				ch <- resp
			}
		}
	}
}

func (sv *supervisor) handleEvent(ev machineEvent) {
	var p struct {
		AppID    string `json:"appId"`
		WSURI    string `json:"wsUri"`
		Message  string `json:"message"`
		Finished bool   `json:"finished"`
		Error    string `json:"error"`
		Log      string `json:"log"`
	}
	_ = json.Unmarshal(ev.Params, &p)
	sv.mu.Lock()
	var started string
	switch ev.Event {
	case "app.start":
		sv.state.AppID = p.AppID
	case "app.debugPort":
		sv.state.VMService = p.WSURI
	case "app.progress":
		if p.Message != "" {
			sv.state.Message = p.Message
		}
	case "app.started":
		sv.state.Status = StatusStarted
		sv.state.Message = "the app is running"
		started = sv.state.VMService
	case "app.stop":
		sv.state.Status = StatusStopped
		if p.Error != "" {
			sv.state.Status = StatusFailed
			sv.state.Message = p.Error
		}
	case "daemon.logMessage":
		if strings.Contains(strings.ToLower(p.Message), "error") {
			sv.state.Message = p.Message
		}
	default:
		sv.mu.Unlock()
		return
	}
	writeState(sv.dir, sv.state)
	sv.mu.Unlock()
	if started != "" && sv.onStarted != nil {
		go sv.onStarted(started)
	}
}

func (sv *supervisor) call(method string, params map[string]any, timeout time.Duration) (machineResponse, error) {
	sv.mu.Lock()
	sv.nextID++
	id := sv.nextID
	ch := make(chan machineResponse, 1)
	sv.calls[id] = ch
	data, _ := json.Marshal([]map[string]any{{"id": id, "method": method, "params": params}})
	_, err := fmt.Fprintf(sv.stdin, "%s\n", data)
	sv.mu.Unlock()
	if err != nil {
		return machineResponse{}, err
	}
	select {
	case resp := <-ch:
		if len(resp.Error) > 0 && string(resp.Error) != "null" {
			return resp, errors.New(strings.Trim(string(resp.Error), `"`))
		}
		return resp, nil
	case <-time.After(timeout):
		return machineResponse{}, fmt.Errorf("no answer to %s within %s", method, timeout)
	}
}

func (sv *supervisor) accept(listener net.Listener) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go sv.serveConn(conn)
	}
}

func (sv *supervisor) serveConn(conn net.Conn) {
	defer conn.Close()
	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		return
	}
	ok, message := sv.command(strings.TrimSpace(line))
	_ = json.NewEncoder(conn).Encode(map[string]any{"ok": ok, "message": message})
}

func (sv *supervisor) command(command string) (bool, string) {
	sv.mu.Lock()
	appID, status := sv.state.AppID, sv.state.Status
	sv.mu.Unlock()
	if status != StatusStarted {
		return false, "the app has not started yet (status " + status + ")"
	}
	switch command {
	case "reload", "restart":
		full := command == "restart"
		resp, err := sv.call("app.restart", map[string]any{"appId": appID, "fullRestart": full, "pause": false, "reason": "manual"}, restartTimeout)
		if err != nil {
			return false, command + " failed: " + err.Error()
		}
		var result struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(resp.Result, &result)
		if result.Code != 0 {
			return false, command + " failed: " + result.Message
		}
		if full && sv.onStarted != nil {
			sv.mu.Lock()
			vm := sv.state.VMService
			sv.mu.Unlock()
			go sv.onStarted(vm)
		}
		if result.Message == "" {
			result.Message = command + " done"
		}
		return true, result.Message
	case "stop":
		if _, err := sv.call("app.stop", map[string]any{"appId": appID}, 30*time.Second); err != nil {
			return false, "stop failed: " + err.Error()
		}
		sv.mu.Lock()
		sv.state.Status = StatusStopped
		sv.state.Message = "stopped by lgrass flutter stop"
		writeState(sv.dir, sv.state)
		sv.mu.Unlock()
		return true, "stopped"
	default:
		return false, "unknown command " + command
	}
}
