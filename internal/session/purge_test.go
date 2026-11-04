// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import "testing"

func seedProject(t *testing.T, s *Store, tab string) {
	t.Helper()
	if err := s.Start("session-" + tab); err != nil {
		t.Fatalf("Start: %v", err)
	}
	if err := s.RegisterTab(tab, "claude"); err != nil {
		t.Fatalf("RegisterTab: %v", err)
	}
	if err := s.SetTabState(tab, "idle"); err != nil {
		t.Fatalf("SetTabState: %v", err)
	}
	if err := s.MarkReady(tab, "plan"); err != nil {
		t.Fatalf("MarkReady: %v", err)
	}
	if _, err := s.AddTip("tip"); err != nil {
		t.Fatalf("AddTip: %v", err)
	}
	if _, err := s.CreateThread(tab, "title", "hello !>>"+tab+"<<!"); err != nil {
		t.Fatalf("CreateThread: %v", err)
	}
}

func countRows(t *testing.T, s *Store, query string, args ...any) int {
	t.Helper()
	var n int
	if err := s.db.QueryRow(query, args...).Scan(&n); err != nil {
		t.Fatalf("%s: %v", query, err)
	}
	return n
}

func TestPurgeProjectDeletesOnlyThatProjectsRows(t *testing.T) {
	gone := openTestStore(t)
	kept, err := Open(DBPath(), "otherproj")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { kept.Close() })

	seedProject(t, gone, "aaaaaaaa-0000-0000-0000-000000000001")
	seedProject(t, kept, "bbbbbbbb-0000-0000-0000-000000000001")
	keptBefore := map[string]int{}
	tables := []string{"lg_messages", "lg_message_mentions", "lg_tab_state", "lg_threads", "lg_tabs", "sessions", "lg_tips", "lg_ready_marks"}
	for _, table := range tables {
		keptBefore[table] = countRows(t, kept, "SELECT COUNT(*) FROM "+table)
	}

	if err := gone.PurgeProject(); err != nil {
		t.Fatalf("PurgeProject: %v", err)
	}

	for _, table := range []string{"sessions", "file_activity", "lg_tips", "lg_tab_sessions", "lg_signatures", "lg_held_calls",
		"lg_tabs", "lg_threads", "lg_notifications", "lg_groups", "lg_ready_marks"} {
		if n := countRows(t, kept, "SELECT COUNT(*) FROM "+table+" WHERE project_id = ?", testProjectID); n != 0 {
			t.Errorf("%s still holds %d rows of the purged project", table, n)
		}
	}
	if n := countRows(t, kept, "SELECT COUNT(*) FROM lg_tab_state WHERE tab_id = 'aaaaaaaa-0000-0000-0000-000000000001'"); n != 0 {
		t.Errorf("lg_tab_state still holds the purged project's tab")
	}
	if n := countRows(t, kept, "SELECT COUNT(*) FROM lg_messages WHERE thread_id NOT IN (SELECT id FROM lg_threads)"); n != 0 {
		t.Errorf("%d messages left without their thread", n)
	}
	if n := countRows(t, kept, "SELECT COUNT(*) FROM lg_message_mentions WHERE message_id NOT IN (SELECT id FROM lg_messages)"); n != 0 {
		t.Errorf("%d mentions left without their message", n)
	}
	for _, table := range tables {
		total := countRows(t, kept, "SELECT COUNT(*) FROM "+table)
		if table == "lg_message_mentions" || table == "lg_messages" || table == "lg_tab_state" {
			if total == 0 && keptBefore[table] > 0 {
				t.Errorf("%s lost the other project's rows", table)
			}
			continue
		}
		if n := countRows(t, kept, "SELECT COUNT(*) FROM "+table+" WHERE project_id = 'otherproj'"); n == 0 && keptBefore[table] > 0 {
			t.Errorf("%s lost the other project's rows", table)
		}
	}
}
