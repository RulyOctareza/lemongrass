// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import "testing"

func TestActorForPrefersTabID(t *testing.T) {
	old := tabID
	t.Cleanup(func() { tabID = old })

	tabID = "tab-42"
	if got := actorFor("short1"); got != "tab-42" {
		t.Errorf("actorFor = %q, want %q", got, "tab-42")
	}

	tabID = ""
	if got := actorFor("short1"); got != "short1" {
		t.Errorf("actorFor with no tab id = %q, want the short id %q", got, "short1")
	}
}
