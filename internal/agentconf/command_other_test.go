// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package agentconf

import "testing"

func TestHookCommandQuotesOnlyWhenNeeded(t *testing.T) {
	for _, c := range []struct{ path, want string }{
		{"/home/u/.lemongrass/bin/lgrassd", "/home/u/.lemongrass/bin/lgrassd hook Stop"},
		{"/home/a b/lgrassd", "'/home/a b/lgrassd' hook Stop"},
		{"/home/it's/lgrassd", `'/home/it'\''s/lgrassd' hook Stop`},
	} {
		if got := HookCommand(c.path, "hook", "Stop"); got != c.want {
			t.Errorf("HookCommand(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}
