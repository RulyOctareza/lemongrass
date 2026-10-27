// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

// Zero overwrites b with zero bytes; not a guarantee, since the runtime may have already copied it elsewhere.
func Zero(b []byte) {
	for i := range b {
		b[i] = 0
	}
}
