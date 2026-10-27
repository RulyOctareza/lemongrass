// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"testing"
	"time"
)

func TestTabReadyForNudgeFollowsTheHookState(t *testing.T) {
	store := openTestStore(t)
	at := time.Now()

	if ready, err := store.TabReadyForNudge(tabA, at); err != nil || !ready {
		t.Errorf("a tab with no state = %v, %v, want ready", ready, err)
	}
	for state, want := range map[string]bool{StateIdle: true, StateWorking: false, StatePrompting: false} {
		if err := store.SetTabState(tabA, state); err != nil {
			t.Fatalf("SetTabState(%s): %v", state, err)
		}
		if ready, _ := store.TabReadyForNudge(tabA, at); ready != want {
			t.Errorf("state %s ready = %v, want %v", state, ready, want)
		}
	}
}

func TestStaleWorkingIsReadyButAPermissionPromptNever(t *testing.T) {
	store := openTestStore(t)
	later := time.Now().Add(2 * workingStaleAfter)

	store.SetTabState(tabA, StateWorking)
	if ready, _ := store.TabReadyForNudge(tabA, later); !ready {
		t.Error("a working state left for a long time should count as ready")
	}
	store.SetTabState(tabA, StatePrompting)
	if ready, _ := store.TabReadyForNudge(tabA, later); ready {
		t.Error("an open permission prompt must never be ready, however long ago")
	}
	store.ClearTabState(tabA)
	if ready, _ := store.TabReadyForNudge(tabA, later); !ready {
		t.Error("a cleared state should count as ready")
	}
}
