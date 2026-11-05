// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package procjob

import (
	"os/exec"
	"testing"
	"time"
)

func TestTerminateEndsTheAssignedProcess(t *testing.T) {
	cmd := exec.Command("ping", "-n", "60", "127.0.0.1")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	job, err := New()
	if err != nil {
		t.Fatal(err)
	}
	defer job.Close()
	if err := job.AssignPID(cmd.Process.Pid); err != nil {
		t.Fatal(err)
	}
	if err := job.Terminate(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("process still running after the job was terminated")
	}
}
