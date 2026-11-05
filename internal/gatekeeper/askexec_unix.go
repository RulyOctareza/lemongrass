// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package gatekeeper

import (
	"os"
	"os/exec"
	"syscall"

	"github.com/creack/pty"
)

type ptyConsole struct {
	*os.File
}

func (c ptyConsole) Resize(rows, cols int) error {
	return pty.Setsize(c.File, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

type unixAskProcess struct {
	cmd  *exec.Cmd
	ptmx *os.File
}

func startAsk(command, dir string, env []string, size askSize) (askProcess, error) {
	shell, err := exec.LookPath("bash")
	if err != nil {
		shell = "/bin/sh"
	}
	cmd := exec.Command(shell, "-c", command)
	cmd.Dir = dir
	cmd.Env = env
	setAskProcAttr(cmd)
	// The terminal start also makes the command a session leader with its own process group.
	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(size.rows), Cols: uint16(size.cols)})
	if err != nil {
		return nil, err
	}
	return &unixAskProcess{cmd: cmd, ptmx: ptmx}, nil
}

func (p *unixAskProcess) console() askConsole { return ptyConsole{p.ptmx} }
func (p *unixAskProcess) wait() error         { return p.cmd.Wait() }
func (p *unixAskProcess) signal(kill bool)    { signalAskGroup(p.cmd, kill) }
func (p *unixAskProcess) exited()             {}
func (p *unixAskProcess) close()              { _ = p.ptmx.Close() }

func (p *unixAskProcess) exitCode(waitErr error) int {
	if p.cmd.ProcessState != nil {
		return p.cmd.ProcessState.ExitCode()
	}
	if waitErr != nil {
		return -1
	}
	return 0
}

func sameEnvName(a, b string) bool { return a == b }

func newAskTail(askSize) askTail { return newTailWriter(askTailLines, askTailLineBytes) }

// The terminal start makes the command a session leader, which also gives it its own process group, so a stop or a timeout reaches everything it started. Setpgid is left off because it cannot be combined with a new session.
func setAskProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{}
	setAskParentDeath(cmd.SysProcAttr)
}

func signalAskGroup(cmd *exec.Cmd, kill bool) {
	if cmd.Process == nil {
		return
	}
	sig := syscall.SIGTERM
	if kill {
		sig = syscall.SIGKILL
	}
	_ = syscall.Kill(-cmd.Process.Pid, sig)
}
