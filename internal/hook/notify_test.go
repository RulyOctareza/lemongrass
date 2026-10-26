// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const (
	notifyTabAuthor = "aaaaaaaa-1111-4111-8111-111111111111"
	notifyTabTarget = "bbbbbbbb-2222-4222-8222-222222222222"
)

func storeWithPendingMention(t *testing.T) *session.Store {
	t.Helper()
	store := openHookTestStore(t)
	store.RegisterTab(notifyTabAuthor, "claude")
	store.RegisterTab(notifyTabTarget, "codex")
	if _, err := store.CreateThread(notifyTabAuthor, "Review", "hi !>>"+notifyTabTarget+"<<!"); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
	return store
}

func TestNotificationContextSurfacesOnceThenMarksSent(t *testing.T) {
	store := storeWithPendingMention(t)

	first := notificationContext(store, notifyTabTarget)
	if len(first) != 1 || !strings.Contains(first[0], ": 1 for you") || strings.Contains(first[0], "hi !>>") {
		t.Fatalf("first = %q, want one content-free nudge", first)
	}
	if again := notificationContext(store, notifyTabTarget); len(again) != 0 {
		t.Errorf("second call repeated the nudge: %q", again)
	}
}

func TestNotificationContextIgnoresTabsWithNothingOrNoTab(t *testing.T) {
	store := storeWithPendingMention(t)

	if got := notificationContext(store, notifyTabAuthor); len(got) != 0 {
		t.Errorf("the author was nudged: %q", got)
	}
	if got := notificationContext(store, ""); len(got) != 0 {
		t.Errorf("a call with no tab id was nudged: %q", got)
	}
}

func TestUserPromptSubmitCarriesHeldNotifications(t *testing.T) {
	store := storeWithPendingMention(t)

	result := hookUserPromptSubmit(store, hookEvent{SessionID: "s", TabID: notifyTabTarget})
	if result.EventName != "UserPromptSubmit" || !strings.Contains(result.AdditionalContext, ": 1 for you") {
		t.Fatalf("UserPromptSubmit = %+v, want the held nudge", result)
	}
	if again := hookUserPromptSubmit(store, hookEvent{SessionID: "s", TabID: notifyTabTarget}); again.hasOutput() {
		t.Errorf("the next prompt repeated the nudge: %q", again.AdditionalContext)
	}
}

func TestPostToolUseAndSessionStartCarryTheNudge(t *testing.T) {
	store := storeWithPendingMention(t)

	result := hookPostToolUse(store, hookEvent{SessionID: "s", ToolName: "Bash", TabID: notifyTabTarget}, t.TempDir())
	if !strings.Contains(result.AdditionalContext, ": 1 for you") {
		t.Errorf("PostToolUse context = %q, want the nudge", result.AdditionalContext)
	}

	store.CreateThread(notifyTabAuthor, "Second", "again !>>"+notifyTabTarget+"<<!")
	start := hookSessionStart(store, hookEvent{SessionID: "s2", TabID: notifyTabTarget}, t.TempDir())
	if !strings.Contains(start.AdditionalContext, ": 1 for you") {
		t.Errorf("SessionStart context = %q, want the pending nudge", start.AdditionalContext)
	}
}

func TestSessionStartTeachesThePrefixToLemongrassTabsOnly(t *testing.T) {
	store := openHookTestStore(t)
	withTab := hookSessionStart(store, hookEvent{SessionID: "s1", TabID: notifyTabTarget}, t.TempDir())
	if !strings.Contains(withTab.AdditionalContext, session.FormatPrefixNote()) {
		t.Errorf("SessionStart context = %q, want the prefix note", withTab.AdditionalContext)
	}
	without := hookSessionStart(store, hookEvent{SessionID: "s2"}, t.TempDir())
	if strings.Contains(without.AdditionalContext, session.Prefix) {
		t.Errorf("SessionStart context without a tab = %q, want no prefix note", without.AdditionalContext)
	}
}
