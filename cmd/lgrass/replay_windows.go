// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package main

import "github.com/faizalv/lemongrass/internal/gitbash"

// A bare "bash" on Windows can resolve to System32\bash.exe, which is WSL and sees another filesystem; the command
// was written for the agent's Git Bash, so it replays there.
func replayShell() (string, error) {
	return gitbash.Bash()
}
