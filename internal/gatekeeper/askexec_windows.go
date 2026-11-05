// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package gatekeeper

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// creack/pty has no Windows terminal, so an ask command fails to start here until the ask terminal gets ConPTY.
func setAskProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NEW_PROCESS_GROUP}
}

// Windows has no signal to ask a process to stop, so both steps kill, and only the direct child: reaching everything
// the command started needs a job object.
func signalAskGroup(cmd *exec.Cmd, kill bool) {
	if cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
