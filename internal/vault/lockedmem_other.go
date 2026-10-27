// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !linux

package vault

// LockKey copies b into a fresh buffer and zeroes b. No OS-level memory locking on this platform.
func LockKey(b []byte) ([]byte, error) {
	if len(b) == 0 {
		return nil, nil
	}
	locked := make([]byte, len(b))
	copy(locked, b)
	Zero(b)
	return locked, nil
}

// LockedFree zeroes memory returned by LockKey.
func LockedFree(b []byte) {
	Zero(b)
}
