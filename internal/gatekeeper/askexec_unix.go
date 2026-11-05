// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package gatekeeper

import (
	"os/exec"
	"syscall"
)

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
