// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const testProjectID = "testproj"

func openTestStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	store, err := Open(DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestLivenessExcludesSelfAndReportsActiveIdling(t *testing.T) {
	store := openTestStore(t)

	if err := store.Start("session-a"); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	if err := store.Start("session-b"); err != nil {
		t.Fatalf("Start b: %v", err)
	}

	// Backdate session-b's activity so it reads as idling.
	old := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := store.db.Exec(
		`UPDATE sessions SET last_activity_at = ? WHERE project_id = ? AND session_id = ?`,
		old, testProjectID, "session-b",
	); err != nil {
		t.Fatalf("backdating session-b: %v", err)
	}

	liveness, err := store.Liveness("session-a", 5*time.Minute)
	if err != nil {
		t.Fatalf("Liveness: %v", err)
	}
	if len(liveness) != 1 {
		t.Fatalf("len(liveness) = %d, want 1 (session-a excluded as self)", len(liveness))
	}
	if liveness[0].SessionID != "session-b" {
		t.Errorf("liveness[0].SessionID = %q, want session-b", liveness[0].SessionID)
	}
	if liveness[0].Active {
		t.Error("session-b backdated 1h with a 5m threshold should read as idling, got Active=true")
	}
}

func TestEndedSessionExcludedFromLiveness(t *testing.T) {
	store := openTestStore(t)

	if err := store.Start("session-a"); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	if err := store.Start("session-b"); err != nil {
		t.Fatalf("Start b: %v", err)
	}
	if err := store.End("session-b"); err != nil {
		t.Fatalf("End b: %v", err)
	}

	liveness, err := store.Liveness("session-a", 5*time.Minute)
	if err != nil {
		t.Fatalf("Liveness: %v", err)
	}
	if len(liveness) != 0 {
		t.Fatalf("len(liveness) = %d, want 0 (session-b has ended)", len(liveness))
	}
}

func TestRecentActivitySameFileAndSameFolder(t *testing.T) {
	store := openTestStore(t)

	if err := store.Start("session-a"); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	if err := store.Start("session-b"); err != nil {
		t.Fatalf("Start b: %v", err)
	}
	if err := store.LogFileActivity("session-b", "/proj/pkg/foo.go"); err != nil {
		t.Fatalf("LogFileActivity: %v", err)
	}

	t.Run("same file", func(t *testing.T) {
		hits, err := store.RecentActivity("session-a", "/proj/pkg/foo.go", 15*time.Minute)
		if err != nil {
			t.Fatalf("RecentActivity: %v", err)
		}
		if len(hits) != 1 || hits[0].SessionID != "session-b" || !hits[0].SameFile {
			t.Fatalf("hits = %+v, want one same-file hit from session-b", hits)
		}
	})

	t.Run("same folder, different file", func(t *testing.T) {
		hits, err := store.RecentActivity("session-a", "/proj/pkg/bar.go", 15*time.Minute)
		if err != nil {
			t.Fatalf("RecentActivity: %v", err)
		}
		if len(hits) != 1 || hits[0].SessionID != "session-b" || hits[0].SameFile {
			t.Fatalf("hits = %+v, want one same-folder (not same-file) hit from session-b", hits)
		}
	})

	t.Run("different folder", func(t *testing.T) {
		hits, err := store.RecentActivity("session-a", "/proj/other/baz.go", 15*time.Minute)
		if err != nil {
			t.Fatalf("RecentActivity: %v", err)
		}
		if len(hits) != 0 {
			t.Fatalf("hits = %+v, want none (different folder)", hits)
		}
	})

	t.Run("excludes the querying session's own activity", func(t *testing.T) {
		if err := store.LogFileActivity("session-a", "/proj/pkg/foo.go"); err != nil {
			t.Fatalf("LogFileActivity: %v", err)
		}
		hits, err := store.RecentActivity("session-a", "/proj/pkg/foo.go", 15*time.Minute)
		if err != nil {
			t.Fatalf("RecentActivity: %v", err)
		}
		for _, h := range hits {
			if h.SessionID == "session-a" {
				t.Fatalf("RecentActivity leaked the querying session's own activity: %+v", hits)
			}
		}
	})
}

func TestRecentActivityExcludesStaleAndEndedSessions(t *testing.T) {
	store := openTestStore(t)

	if err := store.Start("session-a"); err != nil {
		t.Fatalf("Start a: %v", err)
	}
	if err := store.Start("session-b"); err != nil {
		t.Fatalf("Start b: %v", err)
	}
	if err := store.LogFileActivity("session-b", "/proj/pkg/foo.go"); err != nil {
		t.Fatalf("LogFileActivity: %v", err)
	}

	old := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339)
	if _, err := store.db.Exec(
		`UPDATE file_activity SET touched_at = ? WHERE project_id = ? AND session_id = ?`,
		old, testProjectID, "session-b",
	); err != nil {
		t.Fatalf("backdating activity: %v", err)
	}

	hits, err := store.RecentActivity("session-a", "/proj/pkg/foo.go", 15*time.Minute)
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %+v, want none (activity is stale)", hits)
	}

	// Fresh activity, but the session has since ended and is no longer "live".
	if err := store.LogFileActivity("session-b", "/proj/pkg/foo.go"); err != nil {
		t.Fatalf("LogFileActivity: %v", err)
	}
	if err := store.End("session-b"); err != nil {
		t.Fatalf("End b: %v", err)
	}
	hits, err = store.RecentActivity("session-a", "/proj/pkg/foo.go", 15*time.Minute)
	if err != nil {
		t.Fatalf("RecentActivity: %v", err)
	}
	if len(hits) != 0 {
		t.Fatalf("hits = %+v, want none (session-b has ended)", hits)
	}
}

func TestIncrementNudgeCounterFiresAtThresholdThenResets(t *testing.T) {
	store := openTestStore(t)
	if err := store.Start("session-a"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	const threshold = 8
	for i := 1; i < threshold; i++ {
		fire, err := store.IncrementNudgeCounter("session-a", threshold)
		if err != nil {
			t.Fatalf("IncrementNudgeCounter call %d: %v", i, err)
		}
		if fire {
			t.Fatalf("fired early on call %d, want fire only on call %d", i, threshold)
		}
	}

	fire, err := store.IncrementNudgeCounter("session-a", threshold)
	if err != nil {
		t.Fatalf("IncrementNudgeCounter call %d: %v", threshold, err)
	}
	if !fire {
		t.Fatalf("did not fire on call %d", threshold)
	}

	// Counter should have reset, so another threshold-1 calls shouldn't fire again.
	for i := 1; i < threshold; i++ {
		fire, err := store.IncrementNudgeCounter("session-a", threshold)
		if err != nil {
			t.Fatalf("post-reset call %d: %v", i, err)
		}
		if fire {
			t.Fatalf("fired again too early on post-reset call %d", i)
		}
	}
}

func TestIncrementNudgeCounterWithoutPriorStart(t *testing.T) {
	store := openTestStore(t)
	// No Start call. The defensive upsert path should still work.
	fire, err := store.IncrementNudgeCounter("session-a", 1)
	if err != nil {
		t.Fatalf("IncrementNudgeCounter: %v", err)
	}
	if !fire {
		t.Error("threshold of 1 should fire on the first call even without a prior Start")
	}
}

func TestEnsureOpenPreservesExistingSignature(t *testing.T) {
	store := openTestStore(t)
	if err := store.Start("session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.Sign("session-a", "checklist-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.End("session-a"); err != nil {
		t.Fatal(err)
	}
	if err := store.EnsureOpen("session-a"); err != nil {
		t.Fatal(err)
	}
	signedAt, err := store.SignedAt("session-a", "checklist-a")
	if err != nil {
		t.Fatal(err)
	}
	if signedAt.IsZero() {
		t.Error("EnsureOpen cleared the existing signature")
	}
}

func TestTipsAddListRemove(t *testing.T) {
	store := openTestStore(t)

	id, err := store.AddTip("test tip one")
	if err != nil {
		t.Fatalf("AddTip: %v", err)
	}
	if _, err := store.AddTip("test tip two"); err != nil {
		t.Fatalf("AddTip: %v", err)
	}

	tips, err := store.ListTips()
	if err != nil {
		t.Fatalf("ListTips: %v", err)
	}
	if len(tips) != 2 {
		t.Fatalf("len(tips) = %d, want 2", len(tips))
	}
	if tips[0].ID != id || tips[0].Message != "test tip one" {
		t.Errorf("tips[0] = %+v, want {%d test tip one}", tips[0], id)
	}

	if err := store.DeleteTip(id); err != nil {
		t.Fatalf("DeleteTip: %v", err)
	}
	tips, err = store.ListTips()
	if err != nil {
		t.Fatalf("ListTips after delete: %v", err)
	}
	if len(tips) != 1 || tips[0].Message != "test tip two" {
		t.Fatalf("tips after delete = %+v, want only 'test tip two'", tips)
	}
}

func TestSignedAtZeroBeforeSigning(t *testing.T) {
	store := openTestStore(t)

	signedAt, err := store.SignedAt("session-a", "checklist-1")
	if err != nil {
		t.Fatalf("SignedAt: %v", err)
	}
	if !signedAt.IsZero() {
		t.Errorf("SignedAt before Sign = %v, want zero time", signedAt)
	}

	if err := store.Sign("session-a", "checklist-1"); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	signedAt, err = store.SignedAt("session-a", "checklist-1")
	if err != nil {
		t.Fatalf("SignedAt after Sign: %v", err)
	}
	if signedAt.IsZero() {
		t.Error("SignedAt after Sign is still zero")
	}
	if time.Since(signedAt) > time.Minute {
		t.Errorf("SignedAt = %v, expected close to now", signedAt)
	}
}

func TestSignIsPerSessionAndPerChecklist(t *testing.T) {
	store := openTestStore(t)

	if err := store.Sign("session-a", "checklist-1"); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if signedAt, err := store.SignedAt("session-b", "checklist-1"); err != nil || !signedAt.IsZero() {
		t.Errorf("session-b SignedAt = %v, %v, want zero time, nil (signing doesn't cross sessions)", signedAt, err)
	}
	if signedAt, err := store.SignedAt("session-a", "checklist-2"); err != nil || !signedAt.IsZero() {
		t.Errorf("checklist-2 SignedAt = %v, %v, want zero time, nil (signing doesn't cross checklists)", signedAt, err)
	}
}

func TestReSignResetsTheClock(t *testing.T) {
	store := openTestStore(t)

	if err := store.Sign("session-a", "checklist-1"); err != nil {
		t.Fatalf("Sign: %v", err)
	}
	old := time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(
		`UPDATE lg_signatures SET signed_at = ? WHERE project_id = ? AND session_id = ? AND checklist_id = ?`,
		old, testProjectID, "session-a", "checklist-1",
	); err != nil {
		t.Fatalf("backdating signature: %v", err)
	}

	if err := store.Sign("session-a", "checklist-1"); err != nil {
		t.Fatalf("re-Sign: %v", err)
	}
	signedAt, err := store.SignedAt("session-a", "checklist-1")
	if err != nil {
		t.Fatalf("SignedAt: %v", err)
	}
	if time.Since(signedAt) > time.Minute {
		t.Errorf("SignedAt after re-sign = %v, expected close to now, not the backdated value", signedAt)
	}
}

func TestStartClearsSignaturesForReusedSessionID(t *testing.T) {
	store := openTestStore(t)

	if err := store.Sign("session-a", "checklist-1"); err != nil {
		t.Fatalf("Sign: %v", err)
	}

	if err := store.Start("session-a"); err != nil {
		t.Fatalf("Start: %v", err)
	}

	signedAt, err := store.SignedAt("session-a", "checklist-1")
	if err != nil {
		t.Fatalf("SignedAt: %v", err)
	}
	if !signedAt.IsZero() {
		t.Errorf("SignedAt after Start = %v, want zero time (a reused session_id must not inherit a stale sign)", signedAt)
	}
}

func TestRecordTabSessionReplacesTheTabsSession(t *testing.T) {
	store := openTestStore(t)

	if err := store.RecordTabSession("tab-1", "session-a"); err != nil {
		t.Fatalf("RecordTabSession a: %v", err)
	}
	if err := store.RecordTabSession("tab-1", "session-b"); err != nil {
		t.Fatalf("RecordTabSession b: %v", err)
	}
	if err := store.RecordTabSession("tab-2", "session-c"); err != nil {
		t.Fatalf("RecordTabSession c: %v", err)
	}

	tabs, err := store.TabSessions()
	if err != nil {
		t.Fatalf("TabSessions: %v", err)
	}
	if len(tabs) != 2 || tabs["tab-1"] != "session-b" || tabs["tab-2"] != "session-c" {
		t.Errorf("TabSessions = %v, want tab-1=session-b and tab-2=session-c", tabs)
	}
}

func TestTabSessionsAreScopedToTheProject(t *testing.T) {
	store := openTestStore(t)
	other, err := Open(DBPath(), "otherproj")
	if err != nil {
		t.Fatalf("Open other: %v", err)
	}
	t.Cleanup(func() { other.Close() })

	if err := store.RecordTabSession("tab-1", "session-a"); err != nil {
		t.Fatalf("RecordTabSession: %v", err)
	}
	tabs, err := other.TabSessions()
	if err != nil {
		t.Fatalf("TabSessions: %v", err)
	}
	if len(tabs) != 0 {
		t.Errorf("other project sees %v, want none", tabs)
	}
}

func TestForgetTabSessionRemovesOnlyThatTab(t *testing.T) {
	store := openTestStore(t)
	for _, tab := range []string{"tab-1", "tab-2"} {
		if err := store.RecordTabSession(tab, "session-"+tab); err != nil {
			t.Fatalf("RecordTabSession %s: %v", tab, err)
		}
	}

	if err := store.ForgetTabSession("tab-1"); err != nil {
		t.Fatalf("ForgetTabSession: %v", err)
	}
	tabs, _ := store.TabSessions()
	if len(tabs) != 1 || tabs["tab-2"] != "session-tab-2" {
		t.Errorf("after forgetting tab-1, TabSessions = %v", tabs)
	}
}

func TestRegisterTabRecordsVendorAndReplacesIt(t *testing.T) {
	store := openTestStore(t)

	if v, err := store.TabVendor("tab-1"); err != nil || v != "" {
		t.Fatalf("TabVendor before register = %q, %v, want empty, nil", v, err)
	}
	if err := store.RegisterTab("tab-1", "claude"); err != nil {
		t.Fatalf("RegisterTab: %v", err)
	}
	if v, _ := store.TabVendor("tab-1"); v != "claude" {
		t.Errorf("TabVendor = %q, want claude", v)
	}
	if err := store.RegisterTab("tab-1", "codex"); err != nil {
		t.Fatalf("RegisterTab again: %v", err)
	}
	if v, _ := store.TabVendor("tab-1"); v != "codex" {
		t.Errorf("TabVendor after re-register = %q, want codex", v)
	}
}

func TestForgetTabRemovesVendorAndSession(t *testing.T) {
	store := openTestStore(t)

	store.RegisterTab("tab-1", "claude")
	store.RegisterTab("tab-2", "codex")
	store.RecordTabSession("tab-1", "session-a")

	if err := store.ForgetTab("tab-1"); err != nil {
		t.Fatalf("ForgetTab: %v", err)
	}
	if v, _ := store.TabVendor("tab-1"); v != "" {
		t.Errorf("TabVendor(tab-1) = %q, want empty after forget", v)
	}
	if id, _ := store.SessionForTab("tab-1"); id != "" {
		t.Errorf("SessionForTab(tab-1) = %q, want empty after forget", id)
	}
	if v, _ := store.TabVendor("tab-2"); v != "codex" {
		t.Errorf("TabVendor(tab-2) = %q, want codex untouched", v)
	}
}

func TestRegisterTabPrunesStaleRecordsAcrossProjects(t *testing.T) {
	store := openTestStore(t)

	store.RegisterTab("stale-tab", "claude")
	store.RegisterTab("fresh-tab", "claude")
	old := time.Now().Add(-tabRecordTTL - time.Hour).UTC().Format(time.RFC3339Nano)
	if _, err := store.db.Exec(`UPDATE lg_tabs SET updated_at = ? WHERE tab_id = 'stale-tab'`, old); err != nil {
		t.Fatalf("backdating: %v", err)
	}
	if _, err := store.db.Exec(`INSERT INTO lg_tabs (project_id, tab_id, vendor, updated_at) VALUES ('other-project', 'other-stale', 'codex', ?)`, old); err != nil {
		t.Fatalf("seeding other project: %v", err)
	}

	if err := store.RegisterTab("new-tab", "codex"); err != nil {
		t.Fatalf("RegisterTab: %v", err)
	}

	var count int
	store.db.QueryRow(`SELECT COUNT(*) FROM lg_tabs`).Scan(&count)
	if count != 2 {
		t.Errorf("lg_tabs rows = %d, want 2 (fresh-tab and new-tab), stale rows from every project pruned", count)
	}
	if v, _ := store.TabVendor("fresh-tab"); v != "claude" {
		t.Errorf("TabVendor(fresh-tab) = %q, want claude kept", v)
	}
}

func TestTouchTabRefreshesOnlyRegisteredTabs(t *testing.T) {
	store := openTestStore(t)

	store.RegisterTab("tab-1", "claude")
	old := time.Now().Add(-tabRecordTTL + time.Hour).UTC().Format(time.RFC3339Nano)
	store.db.Exec(`UPDATE lg_tabs SET updated_at = ?`, old)

	if err := store.TouchTab("tab-1"); err != nil {
		t.Fatalf("TouchTab: %v", err)
	}
	if err := store.TouchTab("never-registered"); err != nil {
		t.Fatalf("TouchTab unregistered: %v", err)
	}

	var updated string
	store.db.QueryRow(`SELECT updated_at FROM lg_tabs WHERE tab_id = 'tab-1'`).Scan(&updated)
	if updated <= old {
		t.Errorf("updated_at = %q, want later than %q", updated, old)
	}
	if v, _ := store.TabVendor("never-registered"); v != "" {
		t.Errorf("TouchTab created a record for an unregistered tab: vendor %q", v)
	}
}

func TestSessionForTab(t *testing.T) {
	store := openTestStore(t)

	if id, err := store.SessionForTab("tab-1"); err != nil || id != "" {
		t.Fatalf("SessionForTab unknown = %q, %v, want empty, nil", id, err)
	}
	store.RecordTabSession("tab-1", "session-a")
	if id, _ := store.SessionForTab("tab-1"); id != "session-a" {
		t.Errorf("SessionForTab = %q, want session-a", id)
	}
}

func TestOpenDropsLegacyThreadTables(t *testing.T) {
	store := openVersion1Store(t)
	dbPath := DBPath()

	if _, err := store.db.Exec(`CREATE TABLE thread_messages (id INTEGER); PRAGMA user_version = 0`); err != nil {
		t.Fatalf("seeding legacy table: %v", err)
	}
	reopened, err := Open(dbPath, testProjectID)
	if err != nil {
		t.Fatalf("reopen: %v", err)
	}
	defer reopened.Close()

	var name string
	err = reopened.db.QueryRow(`SELECT name FROM sqlite_master WHERE type = 'table' AND name = 'thread_messages'`).Scan(&name)
	if err == nil {
		t.Error("thread_messages still exists after Open")
	}
}

func TestStartWorksOnOlderSessionsTables(t *testing.T) {
	for name, extra := range map[string]string{
		"without the socket columns": "",
		"with the socket columns":    ",\n\t\tmessaging_socket TEXT,\n\t\tmessaging_token TEXT",
	} {
		t.Run(name, func(t *testing.T) {
			t.Setenv("HOME", t.TempDir())
			if err := os.MkdirAll(filepath.Dir(DBPath()), 0o755); err != nil {
				t.Fatalf("MkdirAll: %v", err)
			}
			old, err := sql.Open("sqlite", DBPath())
			if err != nil {
				t.Fatalf("sql.Open: %v", err)
			}
			if _, err := old.Exec(`CREATE TABLE sessions (
		project_id TEXT NOT NULL,
		session_id TEXT NOT NULL,
		started_at TEXT NOT NULL,
		ended_at TEXT,
		last_activity_at TEXT NOT NULL,
		nudge_counter INTEGER NOT NULL DEFAULT 0` + extra + `,
		PRIMARY KEY (project_id, session_id)
	)`); err != nil {
				t.Fatalf("creating the older table: %v", err)
			}
			old.Close()

			store, err := Open(DBPath(), testProjectID)
			if err != nil {
				t.Fatalf("Open: %v", err)
			}
			defer store.Close()
			if err := store.Start("session-a"); err != nil {
				t.Fatalf("Start on an older table: %v", err)
			}
			if err := store.Start("session-a"); err != nil {
				t.Fatalf("second Start on an older table: %v", err)
			}
		})
	}
}

func TestTabTitleIsCleanedCappedAndSurvivesReRegister(t *testing.T) {
	store := openTestStore(t)
	store.RegisterTab(tabA, "claude")

	if err := store.SetTabTitle(tabA, "  fix\nthe\x1b[31m  bug "+strings.Repeat("x", 200)); err != nil {
		t.Fatalf("SetTabTitle: %v", err)
	}
	store.RegisterTab(tabA, "claude")
	titles, err := store.TabTitles()
	if err != nil {
		t.Fatalf("TabTitles: %v", err)
	}
	got := titles[tabA]
	if !strings.HasPrefix(got, "fix the [31m bug x") || len([]rune(got)) != MaxTabTitleRunes {
		t.Errorf("title = %q, want cleaned and capped at %d runes", got, MaxTabTitleRunes)
	}
	if err := store.SetTabTitle(tabB, "unregistered"); err != nil {
		t.Fatalf("SetTabTitle: %v", err)
	}
	if titles, _ := store.TabTitles(); titles[tabB] != "" {
		t.Errorf("an unregistered tab got a record: %q", titles[tabB])
	}
}

func TestListTabsIsOrderedAndShowsVendorAndTitle(t *testing.T) {
	store := openTestStore(t)
	store.RegisterTab(tabB, "codex")
	store.RegisterTab(tabA, "claude")
	store.SetTabTitle(tabA, "fix the bug")

	for i := 0; i < 20; i++ {
		tabs, err := store.ListTabs()
		if err != nil || len(tabs) != 2 {
			t.Fatalf("ListTabs = %+v, %v, want two tabs", tabs, err)
		}
		if tabs[0] != (TabInfo{ID: tabA, Vendor: "claude", Title: "fix the bug"}) || tabs[1].ID != tabB || tabs[1].Title != "" {
			t.Fatalf("ListTabs = %+v, want tabA with its title, then tabB untitled", tabs)
		}
	}
}

func TestClearTabsKeepsSavedSessionsAndClearsTabState(t *testing.T) {
	store := openTestStore(t)
	store.RegisterTab(tabA, "claude")
	store.RecordTabSession(tabA, "sess-1")
	store.SetTabState(tabA, StatePrompting)

	if err := store.ClearTabs(); err != nil {
		t.Fatalf("ClearTabs: %v", err)
	}
	if tabs, _ := store.ListTabs(); len(tabs) != 0 {
		t.Errorf("tabs after clear = %+v, want none", tabs)
	}
	if id, _ := store.SessionForTab(tabA); id != "sess-1" {
		t.Errorf("saved session = %q, want it kept for resume", id)
	}
	if ready, _ := store.TabReadyForNudge(tabA, time.Now()); !ready {
		t.Errorf("the cleared tab's stale prompting state still blocks a nudge")
	}
}

func TestFileActivitySince(t *testing.T) {
	store := openTestStore(t)
	if err := store.LogFileActivity("old", "/p/biblio/a.md"); err != nil {
		t.Fatal(err)
	}
	time.Sleep(10 * time.Millisecond)
	since := time.Now()
	time.Sleep(10 * time.Millisecond)
	store.LogFileActivity("s1", "/p/biblio/a.md")
	store.LogFileActivity("s2", "/p/biblio/b.md")

	touches, err := store.FileActivitySince(since)
	if err != nil {
		t.Fatal(err)
	}
	if len(touches) != 2 || touches[0].SessionID != "s1" || touches[1].SessionID != "s2" {
		t.Fatalf("touches = %+v", touches)
	}
}
