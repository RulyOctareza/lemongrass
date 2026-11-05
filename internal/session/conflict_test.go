// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package session

import (
	"errors"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/testenv"
)

const conflictScope = "conflict:merge:main:feature"

func conflictStore(t *testing.T) *Store {
	t.Helper()
	store := openTestStore(t)
	for _, tab := range []string{tabA, tabB} {
		if err := store.RegisterTab(tab, "claude"); err != nil {
			t.Fatalf("RegisterTab: %v", err)
		}
	}
	return store
}

func ownedRows(t *testing.T, store *Store, tab string) Pending {
	t.Helper()
	p, err := store.PendingForTab(tab)
	if err != nil {
		t.Fatalf("PendingForTab: %v", err)
	}
	return p
}

// A store at version 3 keeps its threads, and a workgroup thread is marked as one.
func TestMigrationV4KindsExistingThreads(t *testing.T) {
	testenv.Home(t)
	saved := migrations
	migrations = migrations[:3]
	store, err := Open(DBPath(), testProjectID)
	migrations = saved
	if err != nil {
		t.Fatalf("Open at v3: %v", err)
	}
	if _, err := store.db.Exec(`
		INSERT INTO lg_threads (id, project_id, title, group_id, created_by, created_at) VALUES ('grp001', 'testproj', 'Group', 7, 'x', '2026-01-01T00:00:00Z');
		INSERT INTO lg_threads (id, project_id, title, created_by, created_at) VALUES ('adh001', 'testproj', 'Adhoc', 'x', '2026-01-01T00:00:00Z');
		INSERT INTO lg_messages (thread_id, tab_id, body, created_at) VALUES ('adh001', 'x', 'old body', '2026-01-01T00:00:00Z');`); err != nil {
		t.Fatalf("seeding: %v", err)
	}
	store.Close()

	store, err = Open(DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer store.Close()
	if v := storeVersion(t, store); v != len(migrations) {
		t.Errorf("version = %d, want %d", v, len(migrations))
	}
	if g, _ := store.ThreadByID("grp001"); g.Kind != ThreadWorkgroup {
		t.Errorf("group thread kind = %q, want workgroup", g.Kind)
	}
	a, _ := store.ThreadByID("adh001")
	if a.Kind != ThreadAdHoc || a.Closed() || a.Scope != "" || a.Assignee != "" {
		t.Errorf("ad hoc thread = %+v, want an open adhoc thread with no scope", a)
	}
	msgs, _, err := store.ReadThread("adh001", 0, 10)
	if err != nil || len(msgs) != 1 || msgs[0].Kind != MessagePlain || msgs[0].Anchor != nil || msgs[0].Files != nil {
		t.Errorf("old message = %+v, %v, want a plain message with no anchor or files", msgs, err)
	}
}

func TestCreateGroupMarksItsThreadWorkgroup(t *testing.T) {
	store := openTestStore(t)
	g, err := store.CreateGroup("Kinds", Member{TabID: tabA, Label: "lead", Vendor: "claude"}, []Member{{TabID: tabB, Label: "one", Vendor: "claude"}})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if th, _ := store.ThreadByID(g.ThreadID); th.Kind != ThreadWorkgroup {
		t.Errorf("kind = %q, want workgroup", th.Kind)
	}
}

func TestOpenConflictReusesTheScopeAndClosesAnEndedOne(t *testing.T) {
	store := conflictStore(t)

	first, err := store.OpenConflict(conflictScope, tabA, `{"a.go":"1"}`)
	if err != nil {
		t.Fatalf("OpenConflict: %v", err)
	}
	if first.Kind != ThreadConflict || first.Scope != conflictScope || first.Assignee != tabA || first.CreatedBy != HumanAuthor || first.Title != "merge:main:feature" {
		t.Errorf("thread = %+v, want an open conflict thread handed to tabA", first)
	}
	again, err := store.OpenConflict(conflictScope, tabB, `{"a.go":"2"}`)
	if err != nil || again.ID != first.ID || again.Assignee != tabB {
		t.Errorf("reopen = %+v, %v, want the same thread handed to tabB", again, err)
	}
	if b, _ := store.ConflictBaseline(first.ID); b != `{"a.go":"2"}` {
		t.Errorf("baseline = %q, want the new hand-off's", b)
	}

	next, err := store.OpenConflict("conflict:rebase:feature:main", tabA, "")
	if err != nil || next.ID == first.ID {
		t.Fatalf("new scope = %+v, %v, want a new thread", next, err)
	}
	if old, _ := store.ThreadByID(first.ID); !old.Closed() {
		t.Error("the thread of the ended scope stayed open")
	}
	open, err := store.OpenConflictThread()
	if err != nil || open.ID != next.ID {
		t.Errorf("open thread = %+v, %v, want only the new one", open, err)
	}
	// sqlite itself holds one open conflict thread per project.
	if _, err := store.db.Exec(`INSERT INTO lg_threads (id, project_id, title, created_by, created_at, kind, scope) VALUES ('dup001', ?, 't', 'human', 'x', 'conflict', 'conflict:x')`, testProjectID); err == nil {
		t.Error("a second open conflict thread was stored")
	}
}

func TestOpenConflictRefusesBadScopeAndUnknownTab(t *testing.T) {
	store := conflictStore(t)
	if _, err := store.OpenConflict("merge:main:feature", tabA, ""); err == nil {
		t.Error("a scope without the conflict prefix was accepted")
	}
	if _, err := store.OpenConflict(conflictScope, tabC, ""); err == nil {
		t.Error("an unregistered tab was handed the conflict")
	}
}

func TestConflictThreadForRefusesOtherTabs(t *testing.T) {
	store := conflictStore(t)
	if _, err := store.ConflictThreadFor(tabA); !errors.Is(err, ErrNoConflict) {
		t.Errorf("no conflict: err = %v, want ErrNoConflict", err)
	}
	th, _ := store.OpenConflict(conflictScope, tabA, "")
	if got, err := store.ConflictThreadFor(tabA); err != nil || got.ID != th.ID {
		t.Errorf("assignee: %+v, %v", got, err)
	}
	var notAssignee NotAssigneeError
	if _, err := store.ConflictThreadFor(tabB); !errors.As(err, &notAssignee) || !strings.Contains(err.Error(), "aaaaaaaa") {
		t.Errorf("other tab: err = %v, want a refusal naming the assignee", err)
	}
	if _, err := store.ConflictThreadFor(""); err == nil {
		t.Error("a call without a tab was accepted")
	}
	store.CloseConflict(conflictScope, th.ID)
	if _, err := store.ConflictThreadFor(tabA); !errors.Is(err, ErrNoConflict) {
		t.Errorf("closed: err = %v, want ErrNoConflict", err)
	}
}

func TestHumanMessagesMentionAndWakeTheAssignee(t *testing.T) {
	store := conflictStore(t)
	th, _ := store.OpenConflict(conflictScope, tabA, "")
	anchor := &Anchor{Path: "a.go", StartLine: 3, EndLine: 4, Quote: "x := 1\ny := 2"}
	msgs, err := store.PostHumanMessages(conflictScope, []HumanMessage{{Body: "keep theirs here", Anchor: anchor}, {Body: "overall fine"}})
	if err != nil || len(msgs) != 2 {
		t.Fatalf("PostHumanMessages: %v, %v", msgs, err)
	}
	read, _, _ := store.ReadThread(th.ID, 0, 10)
	if read[1].TabID != HumanAuthor || read[1].Kind != MessagePlain || read[1].Anchor == nil || *read[1].Anchor != *anchor || read[0].Anchor != nil {
		t.Errorf("stored = %+v, want the human's anchored message then a plain one", read)
	}
	if len(read[0].Mentions) != 1 || read[0].Mentions[0] != tabA {
		t.Errorf("mentions = %v, want the assignee", read[0].Mentions)
	}
	p := ownedRows(t, store, tabA)
	if len(p.Threads) != 1 || p.Threads[0].You.Count != 2 {
		t.Errorf("assignee owed %+v, want 2 for you", p.Threads)
	}
	if !ownedRows(t, store, tabB).Empty() {
		t.Error("a tab other than the assignee was notified")
	}
	if line := FormatNotification(p.Threads, store.Labels([]string{HumanAuthor})); !strings.Contains(line, "2 for you from human") {
		t.Errorf("nudge line = %q", line)
	}

	if _, err := store.PostHumanMessages("conflict:other", []HumanMessage{{Body: "x"}}); err == nil {
		t.Error("a post for another scope was accepted")
	}
	if _, err := store.PostHumanMessages(conflictScope, []HumanMessage{{Body: "x", Anchor: &Anchor{Path: "a.go", StartLine: 5, EndLine: 4}}}); err == nil {
		t.Error("an anchor ending before it starts was accepted")
	}
	if _, err := store.PostHumanMessages(conflictScope, []HumanMessage{{Body: "ok"}, {Body: " "}}); err == nil {
		t.Error("a blank message was accepted")
	}
	if after, _, _ := store.ReadThread(th.ID, 0, 10); len(after) != 2 {
		t.Errorf("a refused batch stored %d messages, want none added", len(after)-2)
	}
}

func TestNotesAndSubmitsWakeNobodyAndOnlyTheAssigneePosts(t *testing.T) {
	store := conflictStore(t)
	th, _ := store.OpenConflict(conflictScope, tabA, "")
	store.PostHumanMessages(conflictScope, []HumanMessage{{Body: "hi"}})

	if _, err := store.PostNote(tabA, Anchor{Path: "a.go", StartLine: 2, EndLine: 2, Quote: "z"}, "kept ours, theirs renamed it"); err != nil {
		t.Fatalf("PostNote: %v", err)
	}
	if _, err := store.PostSubmit(tabA, "Submitted 1 file(s): a.go.", []string{"a.go"}); err != nil {
		t.Fatalf("PostSubmit: %v", err)
	}
	if _, err := store.PostMessage(tabA, th.ID, "which side do you want?"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if !ownedRows(t, store, tabB).Empty() {
		t.Error("a model post in a conflict thread notified a tab")
	}
	msgs, _, _ := store.ReadThread(th.ID, 0, 10)
	if msgs[0].Kind != MessagePlain || msgs[1].Kind != MessageSubmit || len(msgs[1].Files) != 1 || msgs[1].Files[0] != "a.go" || msgs[2].Kind != MessageNote || msgs[2].Anchor.Quote != "z" {
		t.Errorf("messages = %+v", msgs)
	}

	if _, err := store.PostNote(tabB, Anchor{Path: "a.go", StartLine: 1, EndLine: 1}, "x"); err == nil {
		t.Error("a note from another tab was accepted")
	}
	if _, err := store.PostSubmit(tabA, "x", nil); err == nil {
		t.Error("a submission with no files was accepted")
	}
	if _, err := store.PostMessage(tabB, th.ID, "me too"); err == nil {
		t.Error("another tab posted into the conflict thread")
	}
	if _, err := store.PostMessage(tabA, th.ID, "ask !>>bbbbbbbb<<!"); err == nil {
		t.Error("a mention in a conflict thread was accepted")
	}
	store.CloseConflict(conflictScope, th.ID)
	if _, err := store.PostMessage(tabA, th.ID, "late"); err == nil {
		t.Error("a post into a closed conflict thread was accepted")
	}
}

func TestCloseConflictMatchesScopeAndThread(t *testing.T) {
	store := conflictStore(t)
	th, _ := store.OpenConflict(conflictScope, tabA, "")
	if closed, err := store.CloseConflict("conflict:other", ""); err != nil || closed {
		t.Errorf("other scope: %v, %v, want nothing closed", closed, err)
	}
	if closed, _ := store.CloseConflict(conflictScope, "nosuch"); closed {
		t.Error("closed with another thread id")
	}
	if closed, err := store.CloseConflict(conflictScope, th.ID); err != nil || !closed {
		t.Errorf("close: %v, %v", closed, err)
	}
}

func TestThreadReadShowsAnchorsAndKinds(t *testing.T) {
	store := conflictStore(t)
	th, _ := store.OpenConflict(conflictScope, tabA, "")
	store.PostHumanMessages(conflictScope, []HumanMessage{{Body: "why this?", Anchor: &Anchor{Path: "a.go", StartLine: 4, EndLine: 5, Quote: "one\ntwo"}}})
	store.PostNote(tabA, Anchor{Path: "a.go", StartLine: 7, EndLine: 7, Quote: "seven"}, "kept both")
	store.PostSubmit(tabA, "Submitted.", []string{"a.go", "b.go"})
	th, _ = store.ThreadByID(th.ID)
	msgs, more, _ := store.ReadThread(th.ID, 0, 10)
	out := FormatThreadRead(th, msgs, more, false, store.Labels([]string{HumanAuthor, tabA}))
	for _, want := range []string{"opened by human", "human on a.go lines 4-5 (mentions aaaaaaaa):\n> one\n> two\nwhy this?", "aaaaaaaa note on a.go line 7:\n> seven\nkept both", "aaaaaaaa submitted a.go, b.go:\nSubmitted."} {
		if !strings.Contains(out, want) {
			t.Errorf("read missing %q in:\n%s", want, out)
		}
	}
}
