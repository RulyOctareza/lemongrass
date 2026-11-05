// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package privfs

// Restrict is a no-op on Unix: the callers' 0700 and 0600 modes already keep other users out.
func Restrict(path string) error {
	return nil
}
