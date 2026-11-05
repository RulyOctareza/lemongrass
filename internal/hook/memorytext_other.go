// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package hook

import "strings"

// memoryCommandText is the command text the memory gate's patterns read. Off Windows a backslash only escapes, so it is dropped and an escaped path still matches.
func memoryCommandText(command string) string {
	return strings.ReplaceAll(command, "\\", "")
}

// memoryDirText is a memory directory spelled the way memoryCommandText spells the command.
func memoryDirText(dir string) string { return dir }
