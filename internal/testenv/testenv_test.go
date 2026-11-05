// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package testenv

import (
	"os"
	"testing"
)

func TestHomeIsWhatUserHomeDirReturns(t *testing.T) {
	home := Home(t)
	got, err := os.UserHomeDir()
	if err != nil || got != home {
		t.Fatalf("UserHomeDir = %q, %v; want %q", got, err, home)
	}
}
