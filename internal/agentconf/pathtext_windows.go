// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package agentconf

import "strings"

// Windows paths are case-insensitive, so C:\Work and c:\work name the same project and must not get a second table.
func pathTextHas(text, path string) bool {
	return strings.Contains(strings.ToLower(text), strings.ToLower(path))
}
