// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package flutter

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func recorderProcAttr(*exec.Cmd) {}

// interrupt asks scrcpy to finish its recording; it closes the mp4 on SIGINT.
func interrupt(p *os.Process) error {
	return p.Signal(syscall.SIGINT)
}

// Interrupt is the Windows console helper behind lgrass flutter _interrupt; Unix signals scrcpy directly.
func Interrupt(int) error {
	return errors.New("flutter: _interrupt is only used on Windows")
}
