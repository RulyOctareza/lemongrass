// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package vault

import (
	"bytes"
	"testing"
)

// More keys than the default minimum working set can lock, so the growth path in lockPages runs.
func TestLockKeyBeyondTheDefaultWorkingSet(t *testing.T) {
	var keys [][]byte
	defer func() {
		for _, k := range keys {
			LockedFree(k)
		}
	}()
	for i := 0; i < 400; i++ {
		src := bytes.Repeat([]byte{byte(i)}, 32)
		k, err := LockKey(src)
		if err != nil {
			t.Fatalf("key %d: %v", i, err)
		}
		if !bytes.Equal(k, bytes.Repeat([]byte{byte(i)}, 32)) || !bytes.Equal(src, make([]byte, 32)) {
			t.Fatalf("key %d: copy or zeroing wrong", i)
		}
		keys = append(keys, k)
	}
}
