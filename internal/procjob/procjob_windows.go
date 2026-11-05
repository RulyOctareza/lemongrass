// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package procjob

import (
	"fmt"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Job is a kill-on-close job object. Its handle is not inheritable, so only this process holds it.
type Job struct {
	h windows.Handle
}

// New creates an empty kill-on-close job.
func New() (*Job, error) {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return nil, fmt.Errorf("procjob: creating the job: %w", err)
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{}
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	if _, err := windows.SetInformationJobObject(h, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(h)
		return nil, fmt.Errorf("procjob: setting kill on close: %w", err)
	}
	return &Job{h: h}, nil
}

// Assign puts process proc, and every process it starts from then on, into the job.
func (j *Job) Assign(proc windows.Handle) error {
	if err := windows.AssignProcessToJobObject(j.h, proc); err != nil {
		return fmt.Errorf("procjob: assigning the process: %w", err)
	}
	return nil
}

// AssignPID opens process pid and assigns it. A process started without CREATE_SUSPENDED may have started children
// before this runs; those stay outside the job.
func (j *Job) AssignPID(pid int) error {
	proc, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("procjob: opening process %d: %w", pid, err)
	}
	defer windows.CloseHandle(proc)
	return j.Assign(proc)
}

// Terminate ends every process in the job with exit code 1.
func (j *Job) Terminate() error {
	return windows.TerminateJobObject(j.h, 1)
}

// Close releases the job, which ends whatever still runs in it.
func (j *Job) Close() error {
	return windows.CloseHandle(j.h)
}
