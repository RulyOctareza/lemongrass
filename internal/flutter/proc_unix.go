// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package flutter

import (
	"os/exec"
	"syscall"
)

// Signal 0 checks that the process exists without touching it.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// startDetached starts the command in a new session, which keeps the supervisor running after the shell that
// started it exits.
func startDetached(build func() *exec.Cmd) (*exec.Cmd, error) {
	cmd := build()
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd, cmd.Start()
}

// The flutter tool on Unix is a script that execs dart, so killing the one child ends the run.
func killChildrenOnExit() error { return nil }
