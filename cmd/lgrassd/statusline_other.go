// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !linux && !windows

package main

import "os/exec"

// Only Linux has a parent-death signal; elsewhere a cancelled run can leave the human's command to finish on its own.
func runTiedToParent(cmd *exec.Cmd) error {
	return cmd.Run()
}
