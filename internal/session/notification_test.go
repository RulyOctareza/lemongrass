// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"strings"
	"testing"
	"time"
)

func seedTabs(store *Store) {
	store.RegisterTab(tabA, "claude")
	store.RegisterTab(tabB, "codex")
}

func TestMentionCreatesPendingRowForTargetOnly(t *testing.T) {
	store := openTestStore(t)
	seedTabs(store)

	id, err := store.CreateThread(tabA, "Review", "hey !>>"+tabB+"<<! and me !>>"+tabA+"<<!")
	if err != nil {
		t.Fatalf("CreateThread: %v", err)
	}

	pending, err := store.PendingForTab(tabB)
	if err != nil || len(pending) != 1 {
		t.Fatalf("PendingForTab(B) = %+v, %v, want one thread", pending, err)
	}
	p := pending[0]
	if p.ThreadID != id || p.Title != "Review" || p.Count() != 1 || p.You.Count != 1 || len(p.You.Tabs) != 1 || p.You.Tabs[0] != tabA {
		t.Errorf("pending = %+v, want thread %s, one row, sender tabA", p, id)
	}
	if own, _ := store.PendingForTab(tabA); len(own) != 0 {
		t.Errorf("the author was notified of their own mention: %+v", own)
	}
}

func TestPendingCoalescesPerThreadWithDistinctSenders(t *testing.T) {
	store := openTestStore(t)
	seedTabs(store)
	store.RegisterTab(tabC, "codex")

	id, _ := store.CreateThread(tabA, "Review", "one !>>"+tabB+"<<!")
	store.PostMessage(tabA, id, "two !>>"+tabB+"<<!")
	store.PostMessage(tabC, id, "three !>>"+tabB+"<<!")
	other, _ := store.CreateThread(tabA, "Other", "x !>>"+tabB+"<<!")

	pending, _ := store.PendingForTab(tabB)
	if len(pending) != 2 || pending[0].ThreadID != id || pending[1].ThreadID != other {
		t.Fatalf("pending = %+v, want threads %s then %s", pending, id, other)
	}
	if pending[0].Count() != 3 || len(pending[0].You.Tabs) != 2 {
		t.Errorf("first thread: %d rows, %d senders, want 3 rows and 2 distinct senders", pending[0].Count(), len(pending[0].You.Tabs))
	}
}

func TestMarkSentRemovesRowsFromPending(t *testing.T) {
	store := openTestStore(t)
	seedTabs(store)
	store.CreateThread(tabA, "Review", "x !>>"+tabB+"<<!")

	pending, _ := store.PendingForTab(tabB)
	if err := store.MarkNotificationsSent(pending[0].RowIDs); err != nil {
		t.Fatalf("MarkNotificationsSent: %v", err)
	}
	if again, _ := store.PendingForTab(tabB); len(again) != 0 {
		t.Errorf("sent rows still pending: %+v", again)
	}
	var state string
	store.db.QueryRow(`SELECT state FROM lg_notifications LIMIT 1`).Scan(&state)
	if state != "sent" {
		t.Errorf("state = %q, want sent", state)
	}
}

func TestReadingMessagesSettlesOnlyThoseRowsAndKeepsThem(t *testing.T) {
	store := openTestStore(t)
	seedTabs(store)
	id, _ := store.CreateThread(tabA, "Review", "one !>>"+tabB+"<<!")
	second, _ := store.PostMessage(tabA, id, "two !>>"+tabB+"<<!")

	msgs, _, _ := store.ReadThread(id, second.ID, 10)
	var ids []int64
	for _, m := range msgs {
		ids = append(ids, m.ID)
	}
	if err := store.MarkMessagesRead(tabB, ids); err != nil {
		t.Fatalf("MarkMessagesRead: %v", err)
	}

	pending, _ := store.PendingForTab(tabB)
	if len(pending) != 1 || pending[0].Count() != 1 {
		t.Errorf("pending = %+v, want the one unread row", pending)
	}
	var total, read int
	store.db.QueryRow(`SELECT COUNT(*), COALESCE(SUM(state = 'read'), 0) FROM lg_notifications`).Scan(&total, &read)
	if total != 2 || read != 1 {
		t.Errorf("rows total %d read %d, want 2 kept and 1 read", total, read)
	}
}

func TestTabsDueForRetryHonorsAttemptsAndBackoff(t *testing.T) {
	store := openTestStore(t)
	seedTabs(store)
	store.CreateThread(tabA, "Review", "x !>>"+tabB+"<<!")
	backoff := func(attempts int) time.Duration { return time.Duration(attempts) * time.Minute }

	if due, _ := store.TabsDueForRetry(3, backoff, time.Now()); len(due) != 1 {
		t.Fatalf("untried row not due: %v", due)
	}
	pending, _ := store.PendingForTab(tabB)
	store.RecordNotificationAttempt(pending[0].RowIDs)
	if due, _ := store.TabsDueForRetry(3, backoff, time.Now()); len(due) != 0 {
		t.Errorf("row due right after an attempt: %v", due)
	}
	if due, _ := store.TabsDueForRetry(3, backoff, time.Now().Add(2*time.Minute)); len(due) != 1 {
		t.Errorf("row not due after its backoff: %v", due)
	}
	store.RecordNotificationAttempt(pending[0].RowIDs)
	store.RecordNotificationAttempt(pending[0].RowIDs)
	if due, _ := store.TabsDueForRetry(3, backoff, time.Now().Add(time.Hour)); len(due) != 0 {
		t.Errorf("row past its attempt limit still due: %v", due)
	}
	if tabs, _ := store.PendingTabs(); len(tabs) != 1 || tabs[0] != tabB {
		t.Errorf("PendingTabs = %v, want the row past its limit to stay pending for B", tabs)
	}
}

func TestPruneNotificationsKeepsThirtyDays(t *testing.T) {
	store := openTestStore(t)
	seedTabs(store)
	store.CreateThread(tabA, "old", "x !>>"+tabB+"<<!")
	store.CreateThread(tabA, "new", "y !>>"+tabB+"<<!")
	old := time.Now().Add(-NotificationRetention - time.Hour).UTC().Format(time.RFC3339Nano)
	store.db.Exec(`UPDATE lg_notifications SET created_at = ?, state = 'read' WHERE id = 1`, old)

	if err := store.PruneNotifications(time.Now()); err != nil {
		t.Fatalf("PruneNotifications: %v", err)
	}
	var count int
	store.db.QueryRow(`SELECT COUNT(*) FROM lg_notifications`).Scan(&count)
	if count != 1 {
		t.Errorf("rows after prune = %d, want only the recent one", count)
	}
}

func TestVendorForTabFallsBackToGroupMembership(t *testing.T) {
	store := openTestStore(t)
	store.RegisterTab(tabA, "claude")
	store.db.Exec(`INSERT INTO lg_groups (id, project_id, leader_tab_id, created_at) VALUES (1, ?, ?, ?)`, testProjectID, tabA, now())
	store.db.Exec(`INSERT INTO lg_group_members (group_id, tab_id, role, label, vendor) VALUES (1, ?, 'thinker', 'r', 'codex')`, tabB)

	if v, _ := store.VendorForTab(tabA); v != "claude" {
		t.Errorf("VendorForTab(A) = %q, want claude", v)
	}
	if v, _ := store.VendorForTab(tabB); v != "codex" {
		t.Errorf("VendorForTab(B) = %q, want the group member's codex", v)
	}
	if v, _ := store.VendorForTab("nobody"); v != "" {
		t.Errorf("VendorForTab(unknown) = %q, want empty", v)
	}
}

func TestFormatNotificationCoalescesAndCarriesNoContent(t *testing.T) {
	out := FormatNotification([]PendingThread{
		{ThreadID: "t4", Title: "Review", All: KindPart{Count: 2, Tabs: []string{tabA, tabB}}, RowIDs: []int64{1, 2}},
		{ThreadID: "t5", Title: "Other", All: KindPart{Count: 1, Tabs: []string{tabB}}, RowIDs: []int64{3}},
	}, nil)
	for _, want := range []string{"[lg] thread t4: 2 new from aaaaaaaa, bbbbbbbb", "[lg] thread t5: 1 new from bbbbbbbb"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}
