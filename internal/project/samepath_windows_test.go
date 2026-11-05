// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package project

import "testing"

func TestSamePathFoldsCaseOnWindows(t *testing.T) {
	if !samePath(`C:\Users\A\Proj`, `c:\users\a\proj`) {
		t.Fatal("paths differing only in case should match")
	}
	if samePath(`C:\Users\A\proj`, `C:\Users\A\project`) {
		t.Fatal("different paths should not match")
	}
}
