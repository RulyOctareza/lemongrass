// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package hook

import (
	"regexp"
	"strings"
)

// nulRedirect is a redirect to Windows' null device, which reads like any /dev/null redirect.
var nulRedirect = regexp.MustCompile(`(\d*>&?\s*)(nul|//\./nul)\b`)

// memoryCommandText is the command text the memory gate's patterns read. On Windows a backslash is usually a path separator, so it becomes / and the text is lowercased, as the filesystem ignores case: .Claude\Projects\x\memory matches .claude/projects.
func memoryCommandText(command string) string {
	text := strings.ToLower(strings.ReplaceAll(command, "\\", "/"))
	return nulRedirect.ReplaceAllString(text, "${1}/dev/null")
}

// memoryDirText is a memory directory spelled the way memoryCommandText spells the command.
func memoryDirText(dir string) string { return memoryCommandText(dir) }
