// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package flutter

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// stillActive is the exit code Windows reports for a process that has not exited.
const stillActive = 259

func processAlive(pid int) bool {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(h)
	var code uint32
	if err := windows.GetExitCodeProcess(h, &code); err != nil {
		return false
	}
	return code == stillActive
}

// startDetached starts the supervisor with a console of its own that has no window, so neither it nor the console
// programs under it (cmd.exe for flutter.bat, dart, adb) open a window, and its own process group keeps the caller's
// Ctrl+C away from it. It first tries to leave the caller's job object, since an agent shell that runs each command
// in a kill-on-close job would otherwise end the supervisor with the command; a job that forbids leaving refuses the
// start, which is then retried inside the job.
func startDetached(build func() *exec.Cmd) (*exec.Cmd, error) {
	const flags = windows.CREATE_NO_WINDOW | windows.CREATE_NEW_PROCESS_GROUP
	cmd := build()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags | windows.CREATE_BREAKAWAY_FROM_JOB}
	err := cmd.Start()
	if !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return cmd, err
	}
	cmd = build()
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
	return cmd, cmd.Start()
}

// killChildrenOnExit puts the supervisor in a job object that ends every process in it when the supervisor's handle
// closes, which happens when it exits however it exits. Killing flutter.bat alone would leave dart and the build it
// runs going; children started after this call are in the job from their first instruction.
func killChildrenOnExit() error {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return fmt.Errorf("flutter: creating the job object: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("flutter: setting up the job object: %w", err)
	}
	if err := windows.AssignProcessToJobObject(job, windows.CurrentProcess()); err != nil {
		windows.CloseHandle(job)
		return fmt.Errorf("flutter: joining the job object: %w", err)
	}
	// The handle is left open on purpose: it closes when the process exits, which is what ends the job.
	return nil
}
