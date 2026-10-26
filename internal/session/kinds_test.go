// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestGroupMessageKindsFollowMentions(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)

	store.PostMessage(tabA, g.ThreadID, "plain to all")
	for _, tab := range []string{tabB, tabC} {
		pending, _ := store.PendingForTab(tab)
		if len(pending) != 1 || pending[0].All.Count != 1 || pending[0].You.Count != 0 || pending[0].Other.Count != 0 {
			t.Errorf("tab %s pending = %+v, want one all row", TabLabel(tab), pending)
		}
	}

	store.PostMessage(tabA, g.ThreadID, "for one !>>"+tabB+"<<!")
	pendingB, _ := store.PendingForTab(tabB)
	if len(pendingB) != 1 || pendingB[0].You.Count != 1 || pendingB[0].All.Count != 1 {
		t.Errorf("mentioned tab pending = %+v, want one you and one all row", pendingB)
	}
	surfaceC, _ := store.SurfaceableForTab(tabC)
	if len(surfaceC) != 1 || surfaceC[0].Other.Count != 1 || len(surfaceC[0].Other.Tabs) != 1 || surfaceC[0].Other.Tabs[0] != tabB {
		t.Errorf("other tab surfaceable = %+v, want one other row naming the mentioned tab", surfaceC)
	}
	if wake, _ := store.PendingForTab(tabC); len(wake) != 1 || wake[0].Count() != 1 {
		t.Errorf("other tab wake rows = %+v, want only the plain message", wake)
	}
}

func TestOtherRowsNeverWakeATab(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	store.PostMessage(tabA, g.ThreadID, "only for one !>>"+tabB+"<<!")

	tabs, _ := store.PendingTabs()
	if len(tabs) != 1 || tabs[0] != tabB {
		t.Errorf("PendingTabs = %v, want only the mentioned tab", tabs)
	}
	if due, _ := store.TabsDueForRetry(5, func(int) time.Duration { return 0 }, time.Now()); len(due) != 1 || due[0] != tabB {
		t.Errorf("TabsDueForRetry = %v, want only the mentioned tab", due)
	}
	if wake, _ := store.PendingForTab(tabC); len(wake) != 0 {
		t.Errorf("an unmentioned member would be woken: %+v", wake)
	}
}

func TestMarkOtherSentSettlesOnlyOtherRows(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	store.PostMessage(tabA, g.ThreadID, "one !>>"+tabB+"<<!")
	store.PostMessage(tabA, g.ThreadID, "two")

	if err := store.MarkOtherSent(tabC); err != nil {
		t.Fatalf("MarkOtherSent: %v", err)
	}
	surfaced, _ := store.SurfaceableForTab(tabC)
	if len(surfaced) != 1 || surfaced[0].Other.Count != 0 || surfaced[0].All.Count != 1 {
		t.Errorf("surfaceable after MarkOtherSent = %+v, want the all row only", surfaced)
	}
}

func TestOpenAddsKindToAnOlderNotificationsTable(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	if err := os.MkdirAll(filepath.Dir(DBPath()), 0o755); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	old, err := sql.Open("sqlite", DBPath())
	if err != nil {
		t.Fatalf("sql.Open: %v", err)
	}
	for _, stmt := range []string{
		`CREATE TABLE lg_notifications (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			project_id TEXT NOT NULL,
			thread_id TEXT NOT NULL,
			message_id INTEGER NOT NULL,
			target_tab_id TEXT NOT NULL,
			state TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			last_attempt_at TEXT,
			created_at TEXT NOT NULL,
			sent_at TEXT,
			read_at TEXT
		)`,
		`CREATE TABLE lg_threads (id TEXT PRIMARY KEY, project_id TEXT NOT NULL, title TEXT NOT NULL, group_id INTEGER, created_by TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`CREATE TABLE lg_messages (id INTEGER PRIMARY KEY AUTOINCREMENT, thread_id TEXT NOT NULL, tab_id TEXT NOT NULL, body TEXT NOT NULL, created_at TEXT NOT NULL)`,
		`INSERT INTO lg_threads (id, project_id, title, created_by, created_at) VALUES ('old001', 'p', 'Old', 'x', '2026-01-01T00:00:00Z')`,
		`INSERT INTO lg_messages (thread_id, tab_id, body, created_at) VALUES ('old001', 'x', 'hi', '2026-01-01T00:00:00Z')`,
		`INSERT INTO lg_notifications (project_id, thread_id, message_id, target_tab_id, created_at) VALUES ('p', 'old001', 1, '` + tabB + `', '2026-01-01T00:00:00Z')`,
	} {
		if _, err := old.Exec(stmt); err != nil {
			t.Fatalf("seeding the older database: %v", err)
		}
	}
	old.Close()

	store, err := Open(DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	pending, err := store.PendingForTab(tabB)
	if err != nil || len(pending) != 1 || pending[0].All.Count != 1 {
		t.Errorf("pending after the migration = %+v, %v, want the old row as an all row", pending, err)
	}
}

func TestThreadCursorOnlyMovesForwardAndUnreadSkipsOwnMessages(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	if _, seen, _ := store.ThreadCursor(tabB, g.ThreadID); seen {
		t.Fatal("a tab that never read the thread has a cursor")
	}
	m1, _ := store.PostMessage(tabA, g.ThreadID, "one")
	store.AdvanceThreadCursor(tabB, g.ThreadID, m1.ID)
	store.PostMessage(tabB, g.ThreadID, "mine")
	m3, _ := store.PostMessage(tabA, g.ThreadID, "three")
	store.AdvanceThreadCursor(tabB, g.ThreadID, 1)
	if cur, seen, _ := store.ThreadCursor(tabB, g.ThreadID); !seen || cur != m1.ID {
		t.Errorf("cursor = %d, %v, want it to stay at %d", cur, seen, m1.ID)
	}

	msgs, more, err := store.ReadUnread(g.ThreadID, tabB, m1.ID, 10)
	if err != nil || len(msgs) != 1 || msgs[0].ID != m3.ID {
		t.Fatalf("unread = %+v, %v, want only message %d", msgs, err, m3.ID)
	}
	if !more {
		t.Error("more = false, want true since read messages remain older than the page")
	}
	if none, _, _ := store.ReadUnread(g.ThreadID, tabB, m3.ID, 10); len(none) != 0 {
		t.Errorf("unread after the newest = %+v, want none", none)
	}
}

func TestFormatNotificationCarriesKindsOnOneLine(t *testing.T) {
	out := FormatNotification([]PendingThread{{
		ThreadID: "t5",
		You:      KindPart{Count: 1, Tabs: []string{tabA}},
		All:      KindPart{Count: 2, Tabs: []string{tabA, tabC}},
		Other:    KindPart{Count: 1, Tabs: []string{tabB}},
		RowIDs:   []int64{1, 2, 3, 4},
	}}, map[string]string{tabA: "lead", tabB: "reviewer", tabC: "tester"})
	want := "[lg] thread t5: 1 for you from lead; 2 new from lead, tester; 1 for reviewer, not you"
	if out != want {
		t.Errorf("notification = %q, want %q", out, want)
	}
}

func TestFormatThreadReadUsesClockTimesAndDatesOnlyAcrossDays(t *testing.T) {
	thread := Thread{ID: "t7", Title: "Review", CreatedBy: tabA, CreatedAt: "2026-09-25T10:00:00Z", MessageCount: 3}
	msgs := []Message{
		{ID: 9, TabID: tabB, Body: "newest", CreatedAt: "2026-09-26T10:05:09Z"},
		{ID: 8, TabID: tabA, Body: "same day", CreatedAt: "2026-09-26T09:01:00Z"},
		{ID: 7, TabID: tabA, Body: "day before", CreatedAt: "2026-09-25T23:59:00Z"},
	}
	out := FormatThreadRead(thread, msgs, false, true, nil)
	for _, want := range []string{"opened by aaaaaaaa 2026-09-25", "unread only", "times UTC", "#9 10:05 bbbbbbbb", "#8 09:01 aaaaaaaa", "#7 2026-09-25 23:59 aaaaaaaa"} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, ":09") {
		t.Errorf("seconds leaked into the output:\n%s", out)
	}
}

func TestShortGroupAndNothingNewLines(t *testing.T) {
	g := Group{ID: 3, Name: "toy", ThreadID: "t5"}
	short := FormatGroupShort(g, Member{Label: "reviewer", Role: RoleThinker})
	if !strings.HasPrefix(short, Prefix) || strings.Count(short, "\n") != 0 || !strings.Contains(short, "reviewer") {
		t.Errorf("short group line = %q", short)
	}
	if text := FormatNothingNew(Thread{ID: "t5", Title: "toy"}); !strings.Contains(text, "nothing new") || !strings.Contains(text, "--all") {
		t.Errorf("nothing new line = %q", text)
	}
}
