// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import (
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"

	"github.com/faizalv/lemongrass/internal/gitbash"
	"github.com/faizalv/lemongrass/internal/procjob"
)

// Claude Code on Windows runs a status line command through Git Bash, so the chained one does too.
func statusLineCommand(command string) (*exec.Cmd, error) {
	bash, err := gitbash.Bash()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(bash, "-c", command)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
	return cmd, nil
}

// A kill-on-close job stands in for the parent-death signal: when lgrassd exits, its job handle closes and the
// command's tree goes with it. Children bash starts before the assignment escape it; bash starts none that early.
func runTiedToParent(cmd *exec.Cmd) error {
	job, err := procjob.New()
	if err != nil {
		return err
	}
	defer job.Close()
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := job.AssignPID(cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return err
	}
	return cmd.Wait()
}
