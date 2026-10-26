// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"strings"
	"testing"
	"time"
)

const (
	tabCodex  = "cccccccc-4444-4444-8444-444444444444"
	tabCursor = "dddddddd-5555-4555-8555-555555555555"
	tabStale  = "eeeeeeee-6666-4666-8666-666666666666"
	tabQuiet  = "ffffffff-7777-4777-8777-777777777777"
)

// Version 2 turns every old row into a thread row whose ref is its message id, keeping its state.
func TestMigrationToVersion2KeepsLedgerRowsAndReadState(t *testing.T) {
	store := openVersion1Store(t)
	for _, stmt := range []string{
		`INSERT INTO lg_threads (id, project_id, title, created_by, created_at) VALUES ('old001', 'testproj', 'Old', '` + tabA + `', '2026-01-01T00:00:00Z')`,
		`INSERT INTO lg_messages (thread_id, tab_id, body, created_at) VALUES ('old001', '` + tabA + `', 'one', '2026-01-01T00:00:00Z')`,
		`INSERT INTO lg_messages (thread_id, tab_id, body, created_at) VALUES ('old001', '` + tabA + `', 'two', '2026-01-01T00:00:01Z')`,
		`INSERT INTO lg_notifications (project_id, thread_id, message_id, target_tab_id, kind, state, created_at, read_at) VALUES ('testproj', 'old001', 1, '` + tabB + `', 'all', 'read', '2026-01-01T00:00:00Z', '2026-01-01T00:01:00Z')`,
		`INSERT INTO lg_notifications (project_id, thread_id, message_id, target_tab_id, kind, created_at) VALUES ('testproj', 'old001', 2, '` + tabB + `', 'you', '2026-01-01T00:00:01Z')`,
	} {
		if _, err := store.db.Exec(stmt); err != nil {
			t.Fatalf("seeding: %v", err)
		}
	}
	store.Close()

	reopened, err := Open(DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var source, ref, state, readAt string
	if err := reopened.db.QueryRow(`SELECT source, ref, state, read_at FROM lg_notifications WHERE id = 1`).Scan(&source, &ref, &state, &readAt); err != nil {
		t.Fatalf("reading the migrated row: %v", err)
	}
	if source != SourceThread || ref != "1" || state != "read" || readAt != "2026-01-01T00:01:00Z" {
		t.Errorf("migrated row = %s %s %s %s, want thread 1 read with its read_at", source, ref, state, readAt)
	}
	pending, err := reopened.PendingForTab(tabB)
	if err != nil || len(pending.Threads) != 1 || pending.Threads[0].ThreadID != "old001" || pending.Threads[0].You.Count != 1 || len(pending.Events) != 0 {
		t.Fatalf("pending after migration = %+v, %v, want the one unread row for you", pending, err)
	}
	if err := reopened.MarkMessagesRead(tabB, []int64{2}); err != nil {
		t.Fatalf("MarkMessagesRead: %v", err)
	}
	if pending, _ := reopened.PendingForTab(tabB); !pending.Empty() {
		t.Errorf("pending after reading = %+v, want none", pending)
	}
}

func setTabStateAge(t *testing.T, s *Store, tab string, age time.Duration) {
	t.Helper()
	if err := s.SetTabState(tab, StateIdle); err != nil {
		t.Fatalf("SetTabState: %v", err)
	}
	at := time.Now().Add(-age).UTC().Format(time.RFC3339Nano)
	if _, err := s.db.Exec(`UPDATE lg_tab_state SET updated_at = ? WHERE tab_id = ?`, at, tab); err != nil {
		t.Fatalf("aging tab state: %v", err)
	}
}

func TestBranchRowsGoOnlyToRecentlyActiveClaudeAndCodexTabs(t *testing.T) {
	store := openTestStore(t)
	other, err := Open(DBPath(), "otherproj")
	if err != nil {
		t.Fatalf("Open other project: %v", err)
	}
	defer other.Close()

	for tab, vendor := range map[string]string{tabA: "claude", tabCodex: "codex", tabCursor: "cursor", tabStale: "claude", tabQuiet: "codex"} {
		store.RegisterTab(tab, vendor)
	}
	other.RegisterTab(tabB, "claude")
	setTabStateAge(t, store, tabA, time.Minute)
	setTabStateAge(t, store, tabCodex, 4*time.Minute)
	setTabStateAge(t, store, tabCursor, time.Minute)
	setTabStateAge(t, store, tabStale, 6*time.Minute)
	setTabStateAge(t, store, tabB, time.Minute)

	tabs, err := store.InsertBranchChanged(BranchChange{From: "main", To: "feature/x", Carry: CarryCarried}, time.Now())
	if err != nil {
		t.Fatalf("InsertBranchChanged: %v", err)
	}
	if len(tabs) != 2 || tabs[0] != tabA || tabs[1] != tabCodex {
		t.Fatalf("notified tabs = %v, want the claude and codex tabs active in the last 5 minutes", tabs)
	}
	for _, tab := range []string{tabCursor, tabStale, tabQuiet, tabB} {
		if pending, _ := store.PendingForTab(tab); !pending.Empty() {
			t.Errorf("%s got a branch row: %+v", tab, pending)
		}
	}
	pending, _ := store.PendingForTab(tabCodex)
	if len(pending.Events) != 1 || pending.Events[0].Source != SourceBranch || len(pending.Threads) != 0 {
		t.Errorf("codex pending = %+v, want one branch event", pending)
	}
}

func TestBranchChangedRejectsAnUnknownCarry(t *testing.T) {
	store := openTestStore(t)
	if _, err := store.InsertBranchChanged(BranchChange{From: "a", To: "b", Carry: "maybe"}, time.Now()); err == nil {
		t.Error("an unknown carry was accepted")
	}
}

func TestBranchLineNamesTheCarry(t *testing.T) {
	for carry, want := range map[string]string{
		CarryCarried:  "[lg] branch changed from main to fix-1, uncommitted changes carried over. Check git status before you continue.",
		CarryConflict: "[lg] branch changed from main to fix-1, the carried changes conflicted and the stash was kept. Check git status before you continue.",
		CarryNone:     "[lg] branch changed from main to fix-1. Check git status before you continue.",
	} {
		ref := `{"from":"main","to":"fix-1","carry":"` + carry + `"}`
		if got := formatEvent(PendingEvent{Source: SourceBranch, Ref: ref}); got != want {
			t.Errorf("%s line = %q, want %q", carry, got, want)
		}
	}
}

// An idle tab's nudge carries the thread line first, then the branch line, and settles both.
func TestConsumePendingFormatsThreadThenBranchLines(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	if _, err := store.PostMessage(tabA, g.ThreadID, "hello"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	store.RegisterTab(tabB, "claude")
	setTabStateAge(t, store, tabB, time.Minute)
	if _, err := store.InsertBranchChanged(BranchChange{From: "main", To: "fix-1", Carry: CarryNone}, time.Now()); err != nil {
		t.Fatalf("InsertBranchChanged: %v", err)
	}

	text, ok := store.ConsumePending(tabB)
	lines := strings.Split(text, "\n")
	if !ok || len(lines) != 2 || !strings.HasPrefix(lines[0], "[lg] thread "+g.ThreadID) || lines[1] != "[lg] branch changed from main to fix-1. Check git status before you continue." {
		t.Fatalf("ConsumePending = %q, %v, want the thread line then the branch line", text, ok)
	}
	if again, ok := store.ConsumePending(tabB); ok {
		t.Errorf("a second ConsumePending repeated: %q", again)
	}
}
