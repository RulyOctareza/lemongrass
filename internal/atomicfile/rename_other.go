// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package atomicfile

import "os"

// Rename moves oldpath over newpath. On Unix a rename replaces the target even while it is open.
func Rename(oldpath, newpath string) error {
	return os.Rename(oldpath, newpath)
}
