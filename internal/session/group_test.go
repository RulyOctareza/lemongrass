// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

func newTestGroup(t *testing.T, store *Store) Group {
	t.Helper()
	store.RegisterTab(tabA, "claude")
	g, err := store.CreateGroup("Schema review", Member{TabID: tabA, Label: "lead", Vendor: "claude"}, []Member{
		{TabID: tabB, Label: "reviewer", Vendor: "claude"},
		{TabID: tabC, Label: "tester", Vendor: "codex"},
	})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	return g
}

func TestCreateGroupStoresMembersAndItsThread(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)

	members, err := store.GroupMembers(g.ID)
	if err != nil || len(members) != 3 || members[0].Role != RoleLeader || members[0].TabID != tabA {
		t.Fatalf("members = %+v, %v, want the leader first then two thinkers", members, err)
	}
	thread, err := store.ThreadByID(g.ThreadID)
	if err != nil || thread.GroupID != g.ID || thread.Title != "Schema review" || thread.MessageCount != 0 {
		t.Errorf("thread = %+v, %v, want an empty thread owned by the group", thread, err)
	}
	live, err := store.LiveGroupForTab(tabC)
	if err != nil || live.ID != g.ID || !live.Live() {
		t.Errorf("LiveGroupForTab(thinker) = %+v, %v, want the group", live, err)
	}
}

func TestATabCanBelongToOnlyOneLiveGroup(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)

	_, err := store.CreateGroup("other", Member{TabID: tabB, Label: "x", Vendor: "claude"}, []Member{{TabID: "dddddddd-4444-4444-8444-444444444444", Label: "y", Vendor: "codex"}})
	if !errors.Is(err, ErrAlreadyInside) {
		t.Errorf("error = %v, want ErrAlreadyInside for a member creating a group", err)
	}
	if err := store.DisbandGroup(g.ID); err != nil {
		t.Fatalf("DisbandGroup: %v", err)
	}
	if _, err := store.CreateGroup("again", Member{TabID: tabB, Label: "x", Vendor: "claude"}, []Member{{TabID: tabC, Label: "y", Vendor: "codex"}}); err != nil {
		t.Errorf("a disbanded group's tabs cannot form a new one: %v", err)
	}
}

func TestGroupThreadBroadcastsToEveryMemberExceptTheAuthor(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)

	if _, err := store.PostMessage(tabB, g.ThreadID, "status update"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	for tab, want := range map[string]int{tabA: 1, tabB: 0, tabC: 1} {
		pending, _ := threadsOf(store.PendingForTab(tab))
		if len(pending) != want {
			t.Errorf("tab %s pending threads = %d, want %d", TabLabel(tab), len(pending), want)
		}
	}
}

func TestGroupThreadAddsMentionedOutsidersToTheBroadcast(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	outsider := "eeeeeeee-5555-4555-8555-555555555555"
	store.RegisterTab(outsider, "codex")

	if _, err := store.PostMessage(tabA, g.ThreadID, "fyi !>>eeeeeeee<<!"); err != nil {
		t.Fatalf("PostMessage: %v", err)
	}
	if pending, _ := threadsOf(store.PendingForTab(outsider)); len(pending) != 1 {
		t.Errorf("mentioned outsider pending = %d, want 1", len(pending))
	}
	if pending, _ := threadsOf(store.PendingForTab(tabB)); len(pending) != 0 {
		t.Errorf("a member the message is not for would be woken: %+v", pending)
	}
	if pending, _ := threadsOf(store.SurfaceableForTab(tabB)); len(pending) != 1 || pending[0].Other.Count != 1 {
		t.Errorf("member surfaceable = %+v, want one other row", pending)
	}
}

func TestOnlyLiveMembersCanPostToAGroupThread(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	outsider := "eeeeeeee-5555-4555-8555-555555555555"

	if _, err := store.PostMessage(outsider, g.ThreadID, "let me in"); err == nil || !strings.Contains(err.Error(), "only members") {
		t.Errorf("outsider post error = %v, want a members-only refusal", err)
	}
	store.DisbandGroup(g.ID)
	if _, err := store.PostMessage(tabA, g.ThreadID, "still here"); err == nil || !strings.Contains(err.Error(), "disbanded") {
		t.Errorf("post to a disbanded group error = %v, want a disbanded refusal", err)
	}
	if _, _, err := store.ReadThread(g.ThreadID, 0, 10); err != nil {
		t.Errorf("a disbanded group's thread is not readable: %v", err)
	}
}

func TestDisbandGroupTwiceAndUnknown(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)

	if err := store.DisbandGroup(g.ID); err != nil {
		t.Fatalf("first disband: %v", err)
	}
	if err := store.DisbandGroup(g.ID); err == nil || !strings.Contains(err.Error(), "already disbanded") {
		t.Errorf("second disband error = %v", err)
	}
	if err := store.DisbandGroup(999); !errors.Is(err, ErrNoSuchGroup) {
		t.Errorf("unknown disband error = %v, want ErrNoSuchGroup", err)
	}
	if _, err := store.LiveGroupForTab(tabA); !errors.Is(err, ErrNoSuchGroup) {
		t.Errorf("LiveGroupForTab after disband = %v, want ErrNoSuchGroup", err)
	}
}

func TestDeleteGroupRemovesAnUnusedGroupAndKeepsOneWithMessages(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	if err := store.DeleteGroup(g.ID); err != nil {
		t.Fatalf("DeleteGroup: %v", err)
	}
	if _, err := store.GroupByID(g.ID); !errors.Is(err, ErrNoSuchGroup) {
		t.Errorf("group survived delete: %v", err)
	}
	if _, err := store.ThreadByID(g.ThreadID); err != ErrNoSuchThread {
		t.Errorf("thread survived delete: %v", err)
	}
	var members int
	store.db.QueryRow(`SELECT COUNT(*) FROM lg_group_members`).Scan(&members)
	if members != 0 {
		t.Errorf("%d member rows survived delete", members)
	}
}

func TestLabelsUseGroupLabelsAndFallBackToShortIds(t *testing.T) {
	store := openTestStore(t)
	newTestGroup(t, store)
	outsider := "eeeeeeee-5555-4555-8555-555555555555"

	labels := store.Labels([]string{tabA, tabC, outsider})
	if labels[tabA] != "lead" || labels[tabC] != "tester" || labels[outsider] != "eeeeeeee" {
		t.Errorf("labels = %v, want lead, tester and the outsider's short id", labels)
	}
}

func TestNotificationAndReadOutputShowGroupLabels(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	store.PostMessage(tabB, g.ThreadID, "hello team")

	pending, _ := threadsOf(store.PendingForTab(tabA))
	if text := store.NotificationText(Pending{Threads: pending}); !strings.Contains(text, "from reviewer") {
		t.Errorf("notification = %q, want the sender's group label", text)
	}
	msgs, _, _ := store.ReadThread(g.ThreadID, 0, 10)
	thread, _ := store.ThreadByID(g.ThreadID)
	labels := store.Labels([]string{thread.CreatedBy, msgs[0].TabID})
	out := FormatThreadRead(thread, msgs, false, false, labels)
	if !strings.Contains(out, "opened by lead") || !strings.Contains(out, "reviewer:") {
		t.Errorf("read output missing group labels:\n%s", out)
	}
	members, _ := store.GroupMembers(g.ID)
	if header := FormatGroupHeader(g, members); !strings.Contains(header, "lead (leader, claude") || !strings.Contains(header, "tester (thinker, codex") {
		t.Errorf("header = %q", header)
	}
}

func TestFormatMemberHeaderGivesAThinkerItsRoleBack(t *testing.T) {
	leader := FormatMemberHeader(Member{Role: RoleLeader})
	if !strings.Contains(leader, "leader") {
		t.Errorf("leader header = %q", leader)
	}
	header := FormatMemberHeader(Member{Role: RoleThinker, Label: "reviewer", Prompt: "Review the diff.", Skills: []string{"lgrass-connector"}})
	for _, want := range []string{`thinker "reviewer"`, "lgrass-howtobe-thinker, lgrass-connector", "Your assignment:", "Review the diff."} {
		if !strings.Contains(header, want) {
			t.Errorf("header missing %q:\n%s", want, header)
		}
	}
}

func TestLiveGroupsListsOnlyLiveOnesWithTheirThread(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)

	groups, err := store.LiveGroups()
	if err != nil || len(groups) != 1 || groups[0].ID != g.ID || groups[0].ThreadID != g.ThreadID || groups[0].Name != "Schema review" {
		t.Fatalf("LiveGroups = %+v, %v, want the one live group", groups, err)
	}
	if err := store.DisbandGroup(g.ID); err != nil {
		t.Fatalf("disband: %v", err)
	}
	if groups, err := store.LiveGroups(); err != nil || len(groups) != 0 {
		t.Errorf("LiveGroups after disband = %+v, %v, want none", groups, err)
	}
}

func TestGroupNamesAreUniqueEvenAfterADisband(t *testing.T) {
	store := openTestStore(t)
	g := newTestGroup(t, store)
	other := "dddddddd-4444-4444-8444-444444444444"
	another := "eeeeeeee-5555-4555-8555-555555555555"

	if err := store.CheckGroupName("  schema REVIEW "); !errors.Is(err, ErrNameTaken) {
		t.Errorf("CheckGroupName = %v, want ErrNameTaken ignoring case and spaces", err)
	}
	if err := store.CheckGroupName("release audit"); err != nil {
		t.Errorf("CheckGroupName(unused) = %v", err)
	}
	if _, err := store.CreateGroup("SCHEMA review", Member{TabID: other, Label: "x", Vendor: "claude"}, []Member{{TabID: another, Label: "y", Vendor: "codex"}}); !errors.Is(err, ErrNameTaken) {
		t.Errorf("CreateGroup with a taken name = %v, want ErrNameTaken", err)
	}
	if err := store.DisbandGroup(g.ID); err != nil {
		t.Fatalf("DisbandGroup: %v", err)
	}
	if err := store.CheckGroupName("Schema review"); !errors.Is(err, ErrNameTaken) {
		t.Errorf("CheckGroupName after disband = %v, want the name to stay reserved", err)
	}
}

func TestOpenDropsThreadsKeyedByNumber(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lg.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE lg_threads (id INTEGER PRIMARY KEY AUTOINCREMENT, project_id TEXT NOT NULL, title TEXT NOT NULL, group_id INTEGER, created_by TEXT NOT NULL, created_at TEXT NOT NULL);
		INSERT INTO lg_threads (project_id, title, group_id, created_by, created_at) VALUES ('p', 'old', 1, 'tab', 'x')`); err != nil {
		t.Fatal(err)
	}
	db.Close()
	s, err := Open(path, "p")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	g, err := s.CreateGroup("fresh", Member{TabID: "a", Label: "lead", Vendor: "claude"}, []Member{{TabID: "b", Label: "x", Vendor: "claude"}})
	if err != nil {
		t.Fatalf("creating a group after the old threads were dropped: %v", err)
	}
	if len(g.ThreadID) != 6 {
		t.Errorf("thread id = %q, want 6 characters", g.ThreadID)
	}
}
