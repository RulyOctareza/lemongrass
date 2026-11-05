// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package agentconf

import (
	"path/filepath"
	"strings"
)

// ~ is safe inside a Windows path, where it only appears in 8.3 short names such as RUNNER~1, never at the start.
const windowsSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+@:,=~"

// quoteCommandPath writes the path with forward slashes, since Claude Code runs hook commands through Git Bash, where
// an unquoted backslash is lost, or through cmd.exe, which reads both. A path with only safe characters stays
// unquoted, the one form Git Bash, cmd.exe and PowerShell all run; PowerShell reads a quoted first word as a string, not
// a command. Any other path is double-quoted, which Windows paths allow since they cannot contain a double quote.
func quoteCommandPath(path string) string {
	path = filepath.ToSlash(path)
	if path != "" && strings.Trim(path, windowsSafe) == "" {
		return path
	}
	return `"` + path + `"`
}
