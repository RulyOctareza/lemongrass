// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package agentconf

import "testing"

func TestHookCommandQuotesWindowsPaths(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{`C:\Users\ab\.lemongrass\bin\lgrassd.exe`, `C:/Users/ab/.lemongrass/bin/lgrassd.exe hook Stop`},
		{`C:\Users\A B\.lemongrass\bin\lgrassd.exe`, `"C:/Users/A B/.lemongrass/bin/lgrassd.exe" hook Stop`},
	} {
		if got := HookCommand(c.path, "hook", "Stop"); got != c.want {
			t.Errorf("HookCommand(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
