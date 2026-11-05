// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"errors"
	"fmt"
	"os"
	"runtime"
	"slices"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

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
	// askDrainGrace is how long the terminal's last output is waited for after the command exits; a child that keeps the terminal open is not waited for.
	askDrainGrace = 2 * time.Second
)

type askRun struct {
	stop    chan struct{}
	stopped bool
	// term is set once the command has started; the human's view attaches to it.
	term *askTerm
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
func (b *Backend) ExecuteAsk(id string, timeout time.Duration, rows, cols int) (vault.Ask, error) {
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
	go b.runAsk(started, normalizeAskTimeout(timeout), askStartSize(rows, cols), run, startErr)
	if err := <-startErr; err != nil {
		b.runs.mu.Lock()
		delete(b.runs.runs, id)
		b.runs.mu.Unlock()
		b.Asks.Abort(id)
		return vault.Ask{}, fmt.Errorf("gatekeeper: starting the command: %w", err)
	}
	return started, nil
}

type askSize struct {
	rows, cols int
}

// askStartSize is the card terminal's size when the app sent one in range, else the default, so a command's first output already fits the card.
func askStartSize(rows, cols int) askSize {
	if rows < 1 || cols < 1 || rows > askTermMax || cols > askTermMax {
		return askSize{rows: askTermRows, cols: askTermCols}
	}
	return askSize{rows: rows, cols: cols}
}

// askProcess is a started command and the terminal it runs in, behind one interface per OS (askexec_unix.go,
// askexec_windows.go).
type askProcess interface {
	console() askConsole
	wait() error
	// signal asks the command's whole tree to stop, or kills it when kill is set.
	signal(kill bool)
	// exited runs once the command has exited, before its last output is drained: a ConPTY reaches EOF only once it
	// is closed, a pty once every holder has exited.
	exited()
	close()
	exitCode(waitErr error) int
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
func (b *Backend) runAsk(a vault.Ask, timeout time.Duration, size askSize, run *askRun, startErr chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	tail := newTailWriter(askTailLines, askTailLineBytes)
	// Every command gets a terminal, so one that asks for a password, a confirmation or a login can be answered from the card.
	proc, err := startAsk(a.Command, a.Cwd, askEnv(os.Environ(), a.Path, a.AccountEnv), size)
	if err != nil {
		startErr <- err
		return
	}
	term := newAskTerm(proc.console())
	pumped := make(chan struct{})
	go func() {
		term.pump(tail)
		close(pumped)
	}()
	b.runs.mu.Lock()
	run.term = term
	b.runs.mu.Unlock()
	startErr <- nil

	done := make(chan error, 1)
	go func() { done <- proc.wait() }()

	result := vault.AskResult{}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	var waitErr error
	select {
	case waitErr = <-done:
	case <-timer.C:
		result.TimedOut = true
		waitErr = endAskProcess(proc, done)
	case <-run.stop:
		result.Stopped = true
		waitErr = endAskProcess(proc, done)
	}

	proc.exited()
	select {
	case <-pumped:
	case <-time.After(askDrainGrace):
	}
	term.end()
	proc.close()

	result.ExitCode = proc.exitCode(waitErr)
	result.OutputTail = tail.String()
	b.runs.mu.Lock()
	delete(b.runs.runs, a.ID)
	b.runs.mu.Unlock()
	b.Asks.Finish(a.ID, result)
}

// endAskProcess asks the command's tree to stop, then kills it after a grace period.
func endAskProcess(proc askProcess, done <-chan error) error {
	proc.signal(false)
	select {
	case err := <-done:
		return err
	case <-time.After(askStopGrace):
	}
	proc.signal(true)
	return <-done
}

// askEnv is the daemon's environment with the caller's PATH and account variables, so the command finds the same tools and the same Claude or Codex account the tab would. The daemon's own account variables are dropped either way, since the tab's absence of one means the default account. The tab id is dropped, since the command does not run in the tab, and TERM names the terminal the card renders.
func askEnv(base []string, path string, account map[string]string) []string {
	drop := append([]string{"LGRASS_TAB_ID", "TERM"}, vault.AskAccountEnvKeys...)
	if path != "" {
		drop = append(drop, "PATH")
	}
	env := make([]string, 0, len(base)+2+len(account))
	for _, kv := range base {
		name, _, _ := strings.Cut(kv, "=")
		if slices.ContainsFunc(drop, func(d string) bool { return sameEnvName(d, name) }) {
			continue
		}
		env = append(env, kv)
	}
	if path != "" {
		env = append(env, "PATH="+path)
	}
	for _, k := range vault.AskAccountEnvKeys {
		if v := account[k]; v != "" {
			env = append(env, k+"="+v)
		}
	}
	return append(env, "TERM=xterm-256color")
}

// tailWriter keeps the last lines of a command's terminal output as plain text, each cut to a byte cap, so a chatty command cannot grow the daemon's memory. Escape sequences and control bytes are dropped, and a carriage return not followed by a newline restarts the line, so a progress bar leaves only its last state.
type tailWriter struct {
	mu       sync.Mutex
	max      int
	lineCap  int
	lines    []string
	partial  []byte
	overflow bool
	esc      escState
	cr       bool
}

type escState int

const (
	escNone escState = iota
	// escStart follows ESC.
	escStart
	// escIntermediate follows ESC and an intermediate byte, such as a charset pick, up to its final byte.
	escIntermediate
	// escCSI runs from ESC [ to a final byte.
	escCSI
	// escString runs from ESC ], P, X, ^ or _ to BEL or ESC \.
	escString
	// escStringEsc follows an ESC inside a string, which ends it when a backslash comes next.
	escStringEsc
)

func newTailWriter(maxLines, lineCap int) *tailWriter {
	return &tailWriter{max: maxLines, lineCap: lineCap}
}

func (w *tailWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, c := range p {
		w.feed(c)
	}
	return len(p), nil
}

func (w *tailWriter) feed(c byte) {
	switch w.esc {
	case escStart:
		switch {
		case c == '[':
			w.esc = escCSI
		case c == ']' || c == 'P' || c == 'X' || c == '^' || c == '_':
			w.esc = escString
		case c >= 0x20 && c <= 0x2f:
			w.esc = escIntermediate
		default:
			w.esc = escNone
		}
		return
	case escIntermediate:
		if c >= 0x30 && c <= 0x7e {
			w.esc = escNone
		}
		return
	case escCSI:
		if c >= 0x40 && c <= 0x7e {
			w.esc = escNone
		}
		return
	case escString:
		if c == 0x07 {
			w.esc = escNone
		} else if c == 0x1b {
			w.esc = escStringEsc
		}
		return
	case escStringEsc:
		if c == '\\' {
			w.esc = escNone
		} else if c != 0x1b {
			w.esc = escString
		}
		return
	}
	if w.cr {
		w.cr = false
		if c == '\n' {
			w.push()
			return
		}
		w.partial = w.partial[:0]
		w.overflow = false
	}
	switch {
	case c == '\n':
		w.push()
	case c == '\r':
		w.cr = true
	case c == 0x1b:
		w.esc = escStart
	case c == 0x08:
		if len(w.partial) > 0 {
			_, size := utf8.DecodeLastRune(w.partial)
			w.partial = w.partial[:len(w.partial)-size]
		}
	case (c < 0x20 && c != '\t') || c == 0x7f:
	case len(w.partial) < w.lineCap:
		w.partial = append(w.partial, c)
	default:
		w.overflow = true
	}
}

func (w *tailWriter) push() {
	line := string(w.partial)
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
