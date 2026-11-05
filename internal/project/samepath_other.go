// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package project

// samePath compares two resolved paths exactly.
func samePath(a, b string) bool {
	return a == b
}
