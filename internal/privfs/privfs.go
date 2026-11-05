// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package privfs keeps lemongrass's own directories readable by the current user only. On Unix that is the 0700
// mode the callers already used; on Windows mode bits only toggle read-only, so the directory gets an explicit DACL.
package privfs

import "os"

// MkdirAll creates path like os.MkdirAll with mode 0700 and then restricts it to the current user.
func MkdirAll(path string) error {
	if err := os.MkdirAll(path, 0o700); err != nil {
		return err
	}
	return Restrict(path)
}
