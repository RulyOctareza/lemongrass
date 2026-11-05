// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package ipc

// SameExecutable reports whether two binary paths name the same file.
func SameExecutable(a, b string) bool {
	return a == b
}
