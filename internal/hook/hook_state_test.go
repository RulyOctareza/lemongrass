// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
)

// Tells the three states apart: idle is ready now, working is ready only once stale, prompting never is.
func stateOf(t *testing.T, store *session.Store, tab string) string {
	t.Helper()
	now, err := store.TabReadyForNudge(tab, time.Now())
	if err != nil {
		t.Fatalf("TabReadyForNudge: %v", err)
	}
	stale, _ := store.TabReadyForNudge(tab, time.Now().Add(time.Hour))
	switch {
	case now:
		return session.StateIdle
	case stale:
		return session.StateWorking
	}
	return session.StatePrompting
}

func TestTabStateFollowsTheAgentsHookEvents(t *testing.T) {
	store := openHookTestStore(t)
	tab := notifyTabTarget
	steps := []struct {
		event   string
		payload hookEvent
		want    string
	}{
		{"SessionStart", hookEvent{}, session.StateIdle},
		{"UserPromptSubmit", hookEvent{}, session.StateWorking},
		{"PreToolUse", hookEvent{}, session.StateWorking},
		{"Notification", hookEvent{NotificationType: "permission_prompt"}, session.StatePrompting},
		{"PostToolUse", hookEvent{}, session.StateWorking},
		{"Stop", hookEvent{}, session.StateIdle},
		{"UserPromptSubmit", hookEvent{}, session.StateWorking},
		{"PermissionRequest", hookEvent{}, session.StatePrompting},
		{"PostToolUse", hookEvent{}, session.StateWorking},
		{"Stop", hookEvent{}, session.StateIdle},
		{"UserPromptSubmit", hookEvent{}, session.StateWorking},
		{"PermissionRequest", hookEvent{}, session.StatePrompting},
		{"Interrupt", hookEvent{}, session.StateIdle},
		{"Notification", hookEvent{Message: "Claude needs your permission to use Bash"}, session.StatePrompting},
		{"Notification", hookEvent{NotificationType: "idle_prompt"}, session.StateIdle},
		{"Notification", hookEvent{NotificationType: "auth_success"}, session.StateIdle},
	}
	for _, step := range steps {
		step.payload.TabID = tab
		recordTabState(store, step.event, step.payload)
		if got := stateOf(t, store, tab); got != step.want {
			t.Fatalf("after %s the state is %s, want %s", step.event, got, step.want)
		}
	}

	recordTabState(store, "PreToolUse", hookEvent{TabID: tab})
	recordTabState(store, "SessionEnd", hookEvent{TabID: tab})
	if got := stateOf(t, store, tab); got != session.StateIdle {
		t.Errorf("after SessionEnd the state is %s, want it cleared", got)
	}
}

func TestTabStateIgnoresSessionsWithoutATab(t *testing.T) {
	store := openHookTestStore(t)
	recordTabState(store, "PreToolUse", hookEvent{})
	if got := stateOf(t, store, notifyTabTarget); got != session.StateIdle {
		t.Errorf("a hook with no tab changed a tab's state to %s", got)
	}
}

func TestConsumePendingReturnsTheNudgeOnceAndMarksItSent(t *testing.T) {
	store := storeWithPendingMention(t)
	text, ok := store.ConsumePending(notifyTabTarget)
	if !ok || !strings.HasPrefix(text, "[lg] thread ") || !strings.Contains(text, ": 1 for you from aaaaaaaa") || strings.Contains(text, "Review") {
		t.Fatalf("ConsumePending = %q, %v, want a title-free nudge", text, ok)
	}
	if again, ok := store.ConsumePending(notifyTabTarget); ok {
		t.Errorf("a second call returned %q, want nothing pending", again)
	}
}
