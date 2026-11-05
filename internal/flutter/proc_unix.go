// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package flutter

import "syscall"

// Signal 0 checks that the process exists without touching it.
func processAlive(pid int) bool {
	return syscall.Kill(pid, 0) == nil
}

// A new session keeps the supervisor running after the shell that started it exits.
func detachedProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setsid: true}
}
