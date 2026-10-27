// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const (
	kindsLeader = "aaaaaaaa-1111-4111-8111-111111111111"
	kindsOne    = "bbbbbbbb-2222-4222-8222-222222222222"
	kindsTwo    = "cccccccc-3333-4333-8333-333333333333"
)

func storeWithKindsGroup(t *testing.T) (*session.Store, session.Group) {
	t.Helper()
	store := openHookTestStore(t)
	store.RegisterTab(kindsLeader, "claude")
	g, err := store.CreateGroup("Review", session.Member{TabID: kindsLeader, Label: "lead", Vendor: "claude"}, []session.Member{
		{TabID: kindsOne, Label: "reviewer", Vendor: "claude"},
		{TabID: kindsTwo, Label: "tester", Vendor: "claude"},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	return store, g
}

func TestHookSurfacesNotYouLinesButTheNudgeDoesNot(t *testing.T) {
	store, g := storeWithKindsGroup(t)
	store.PostMessage(kindsLeader, g.ThreadID, "for one !>>"+kindsOne+"<<!")

	if text, ok := store.ConsumePending(kindsTwo); ok {
		t.Fatalf("a nudge for an unmentioned member was composed: %q", text)
	}
	got := notificationContext(store, kindsTwo)
	if len(got) != 1 || !strings.Contains(got[0], "1 for reviewer, not you") {
		t.Fatalf("hook context = %q, want the not you line", got)
	}
	if again := notificationContext(store, kindsTwo); len(again) != 0 {
		t.Errorf("the not you line repeated: %q", again)
	}
}

func TestNudgeSettlesTheNotYouRowsOfThatTab(t *testing.T) {
	store, g := storeWithKindsGroup(t)
	store.PostMessage(kindsOne, g.ThreadID, "to lead !>>"+kindsLeader+"<<!")
	store.PostMessage(kindsLeader, g.ThreadID, "to all")

	text, ok := store.ConsumePending(kindsTwo)
	if !ok || strings.Contains(text, "not you") || !strings.Contains(text, "1 new from lead") {
		t.Fatalf("nudge = %q, %v, want the plain message only", text, ok)
	}
	if got := notificationContext(store, kindsTwo); len(got) != 0 {
		t.Errorf("the not you row surfaced after the nudge: %q", got)
	}
}
