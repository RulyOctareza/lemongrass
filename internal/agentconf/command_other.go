// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package agentconf

import "strings"

const shellSafe = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789/._-+@%:,="

// quoteCommandPath single-quotes the path for sh only when it needs it, so the usual ~/.lemongrass/bin/lgrassd
// command stays exactly as earlier keepers wrote it and no settings file is rewritten for the change.
func quoteCommandPath(path string) string {
	if path != "" && strings.Trim(path, shellSafe) == "" {
		return path
	}
	return "'" + strings.ReplaceAll(path, "'", `'\''`) + "'"
}
