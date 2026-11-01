// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/vault"
)

// An executed command runs asynchronously in the daemon so an image pull or a large install can outlast any socket deadline: execute_ask starts it and returns, and the result answers the ask when it ends. The human picks the timeout on the card.
const (
	DefaultAskTimeout = 10 * time.Minute
	MaxAskTimeout     = 4 * time.Hour
	askTailLines      = 50
	askTailLineBytes  = 4000
	// askStopGrace is how long a stopped or timed-out command gets after SIGTERM before its process group is killed.
	askStopGrace = 5 * time.Second
)

type askRun struct {
	stop    chan struct{}
	stopped bool
}

type askRuns struct {
	mu   sync.Mutex
	runs map[string]*askRun
}

func newAskRuns() *askRuns {
	return &askRuns{runs: map[string]*askRun{}}
}

func normalizeAskTimeout(timeout time.Duration) time.Duration {
	if timeout <= 0 {
		return DefaultAskTimeout
	}
	if timeout > MaxAskTimeout {
		return MaxAskTimeout
	}
	return timeout
}

// ExecuteAsk starts a waiting command ask's command in the ask's working directory, as the user, with the caller's PATH. The guard's deny and ask rules do not apply to a command the human executes; the never-execute list does.
func (b *Backend) ExecuteAsk(id string, timeout time.Duration) (vault.Ask, error) {
	a, err := b.Asks.Get(id)
	if err != nil {
		return vault.Ask{}, err
	}
	if a.Type != vault.AskCommand {
		return vault.Ask{}, fmt.Errorf("gatekeeper: ask %s is a %s ask, only a command ask is executed", id, a.Type)
	}
	home, _ := os.UserHomeDir()
	if reason, refused := guard.NeverExecute(a.Command, a.Cwd, home); refused {
		return vault.Ask{}, fmt.Errorf("gatekeeper: lemongrass never executes this command: %s", reason)
	}
	if info, err := os.Stat(a.Cwd); err != nil || !info.IsDir() {
		return vault.Ask{}, fmt.Errorf("gatekeeper: the ask's working directory %s is not a directory", a.Cwd)
	}
	started, err := b.Asks.Start(id)
	if err != nil {
		return vault.Ask{}, err
	}
	run := &askRun{stop: make(chan struct{})}
	b.runs.mu.Lock()
	b.runs.runs[id] = run
	b.runs.mu.Unlock()

	startErr := make(chan error, 1)
	go b.runAsk(started, normalizeAskTimeout(timeout), run, startErr)
	if err := <-startErr; err != nil {
		b.runs.mu.Lock()
		delete(b.runs.runs, id)
		b.runs.mu.Unlock()
		b.Asks.Abort(id)
		return vault.Ask{}, fmt.Errorf("gatekeeper: starting the command: %w", err)
	}
	return started, nil
}

// StopAsk ends a running command; its result is marked stopped.
func (b *Backend) StopAsk(id string) error {
	b.runs.mu.Lock()
	defer b.runs.mu.Unlock()
	run := b.runs.runs[id]
	if run == nil {
		return errors.New("gatekeeper: no command is running for that ask")
	}
	if !run.stopped {
		run.stopped = true
		close(run.stop)
	}
	return nil
}

// runAsk owns the command from start to finish. It holds its OS thread for the whole run because the parent-death signal is tied to the thread that started the child, and a thread Go retires early would kill the command with it.
func (b *Backend) runAsk(a vault.Ask, timeout time.Duration, run *askRun, startErr chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	shell, err := exec.LookPath("bash")
	if err != nil {
		shell = "/bin/sh"
	}
	tail := newTailWriter(askTailLines, askTailLineBytes)
	cmd := exec.Command(shell, "-c", a.Command)
	cmd.Dir = a.Cwd
	cmd.Env = askEnv(os.Environ(), a.Path)
	cmd.Stdout = tail
	cmd.Stderr = tail
	setAskProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		startErr <- err
		return
	}
	startErr <- nil

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	result := vault.AskResult{}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-timer.C:
		result.TimedOut = true
		waitErr = endAskProcess(cmd, done)
	case <-run.stop:
		result.Stopped = true
		waitErr = endAskProcess(cmd, done)
	}

	result.ExitCode = exitCode(cmd, waitErr)
	result.OutputTail = tail.String()
	b.runs.mu.Lock()
	delete(b.runs.runs, a.ID)
	b.runs.mu.Unlock()
	b.Asks.Finish(a.ID, result)
}

// endAskProcess asks the command's process group to stop, then kills it after a grace period.
func endAskProcess(cmd *exec.Cmd, done <-chan error) error {
	signalAskGroup(cmd, false)
	select {
	case err := <-done:
		return err
	case <-time.After(askStopGrace):
	}
	signalAskGroup(cmd, true)
	return <-done
}

func exitCode(cmd *exec.Cmd, err error) int {
	if cmd.ProcessState != nil {
		return cmd.ProcessState.ExitCode()
	}
	if err != nil {
		return -1
	}
	return 0
}

// askEnv is the daemon's environment with the caller's PATH, so the command finds the same tools the tab would. The tab id is dropped, since the command does not run in the tab.
func askEnv(base []string, path string) []string {
	env := make([]string, 0, len(base)+1)
	for _, kv := range base {
		if path != "" && strings.HasPrefix(kv, "PATH=") {
			continue
		}
		if strings.HasPrefix(kv, "LGRASS_TAB_ID=") {
			continue
		}
		env = append(env, kv)
	}
	if path != "" {
		env = append(env, "PATH="+path)
	}
	return env
}

// tailWriter keeps the last lines of a command's output, each cut to a byte cap, so a chatty command cannot grow the daemon's memory.
type tailWriter struct {
	mu       sync.Mutex
	max      int
	lineCap  int
	lines    []string
	partial  []byte
	overflow bool
}

func newTailWriter(maxLines, lineCap int) *tailWriter {
	return &tailWriter{max: maxLines, lineCap: lineCap}
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range p {
		if c == '\n' {
			w.push()
			continue
		}
		if len(w.partial) < w.lineCap {
			w.partial = append(w.partial, c)
		} else {
			w.overflow = true
		}
	}
	return len(p), nil
}

func (w *tailWriter) push() {
	line := strings.TrimSuffix(string(w.partial), "\r")
	if w.overflow {
		line += " [line cut]"
	}
	w.lines = append(w.lines, line)
	if len(w.lines) > w.max {
		w.lines = w.lines[len(w.lines)-w.max:]
	}
	w.partial = w.partial[:0]
	w.overflow = false
}

func (w *tailWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	lines := append([]string(nil), w.lines...)
	if len(w.partial) > 0 {
		lines = append(lines, string(w.partial))
		if len(lines) > w.max {
			lines = lines[len(lines)-w.max:]
		}
	}
	return strings.Join(lines, "\n")
}
