// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package main

import "os/exec"

func statusLineCommand(command string) (*exec.Cmd, error) {
	return exec.Command("sh", "-c", command), nil
}
