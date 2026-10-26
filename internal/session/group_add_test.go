// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"errors"
	"strings"
	"testing"
)

const (
	tabAdded     = "dddddddd-4444-4444-8444-444444444444"
	tabAdded2    = "eeeeeeee-5555-4555-8555-555555555555"
	tabElsewhere = "ffffffff-6666-4666-8666-666666666666"
)

func TestAddMembersJoinsALiveGroup(t *testing.T) {
	store := openTestStore(t)
	g, err := store.CreateGroup("g", Member{TabID: tabA, Label: "lead", Vendor: "claude"}, []Member{{TabID: tabB, Label: "reviewer", Vendor: "claude"}})
	if err != nil {
		t.Fatalf("CreateGroup: %v", err)
	}
	if err := store.AddMembers(g.ID, []Member{{TabID: tabAdded, Label: "docs", Vendor: "codex", Prompt: "Write docs.", Skills: []string{"lgrass-connector"}}}); err != nil {
		t.Fatalf("AddMembers: %v", err)
	}
	group, member, err := store.LiveMembership(tabAdded)
	if err != nil || group.ID != g.ID || member.Role != RoleThinker || member.Prompt != "Write docs." || len(member.Skills) != 1 {
		t.Errorf("LiveMembership = %+v, %+v, %v", group, member, err)
	}
	if members, _ := store.GroupMembers(g.ID); len(members) != 3 {
		t.Errorf("members = %d, want 3", len(members))
	}

	if err := store.DeleteMembers(g.ID, []string{tabAdded, tabA}); err != nil {
		t.Fatalf("DeleteMembers: %v", err)
	}
	if members, _ := store.GroupMembers(g.ID); len(members) != 2 {
		t.Errorf("after DeleteMembers members = %d, want the leader and reviewer kept", len(members))
	}
}

func TestAddMembersRefusesClashesAndDisbandedGroups(t *testing.T) {
	store := openTestStore(t)
	g, _ := store.CreateGroup("g", Member{TabID: tabA, Label: "lead", Vendor: "claude"}, []Member{{TabID: tabB, Label: "reviewer", Vendor: "claude"}})
	other, _ := store.CreateGroup("other", Member{TabID: tabC, Label: "lead", Vendor: "claude"}, []Member{{TabID: tabElsewhere, Label: "x", Vendor: "claude"}})

	if err := store.AddMembers(g.ID, []Member{{TabID: tabAdded, Label: "Reviewer", Vendor: "claude"}}); !errors.Is(err, ErrLabelTaken) {
		t.Errorf("label clash = %v, want ErrLabelTaken", err)
	}
	if err := store.AddMembers(g.ID, []Member{{TabID: tabAdded, Label: "a", Vendor: "claude"}, {TabID: tabAdded2, Label: "A", Vendor: "claude"}}); !errors.Is(err, ErrLabelTaken) {
		t.Errorf("clash inside the addition = %v, want ErrLabelTaken", err)
	}
	if err := store.AddMembers(g.ID, []Member{{TabID: tabElsewhere, Label: "y", Vendor: "claude"}}); !errors.Is(err, ErrAlreadyInside) {
		t.Errorf("tab of another live group = %v, want ErrAlreadyInside", err)
	}
	if members, _ := store.GroupMembers(g.ID); len(members) != 2 {
		t.Errorf("a refused addition left %d members, want 2", len(members))
	}

	store.DisbandGroup(other.ID)
	if err := store.AddMembers(other.ID, []Member{{TabID: tabAdded, Label: "z", Vendor: "claude"}}); err == nil {
		t.Error("a disbanded group accepted a member")
	}
	if err := store.AddMembers(9999, []Member{{TabID: tabAdded, Label: "z", Vendor: "claude"}}); !errors.Is(err, ErrNoSuchGroup) {
		t.Errorf("unknown group = %v, want ErrNoSuchGroup", err)
	}
}

func TestAddCollaboratorJoinsWithItsOwnRoleAndNoAssignment(t *testing.T) {
	store := openTestStore(t)
	g, _ := store.CreateGroup("g", Member{TabID: tabA, Label: "lead", Vendor: "claude"}, []Member{{TabID: tabB, Label: "reviewer", Vendor: "claude"}})
	if err := store.AddCollaborator(g.ID, Member{TabID: tabAdded, Label: "research", Vendor: "codex", Prompt: "ignored", Skills: []string{"x"}}); err != nil {
		t.Fatalf("AddCollaborator: %v", err)
	}
	_, m, err := store.LiveMembership(tabAdded)
	if err != nil || m.Role != RoleCollaborator || m.Prompt != "" || len(m.Skills) != 0 {
		t.Errorf("member = %+v, %v", m, err)
	}
	if err := store.AddCollaborator(g.ID, Member{TabID: tabAdded2, Label: "Reviewer", Vendor: "claude"}); !errors.Is(err, ErrLabelTaken) {
		t.Errorf("label clash = %v, want ErrLabelTaken", err)
	}
	if h := FormatMemberHeader(m); !strings.Contains(h, `collaborator "research"`) || !strings.Contains(h, CollaboratorJoinCommand) {
		t.Errorf("header = %q", h)
	}
}

func TestSystemMessageNotifiesEveryMemberTheLeaderIncluded(t *testing.T) {
	store := openTestStore(t)
	g, _ := store.CreateGroup("g", Member{TabID: tabA, Label: "lead", Vendor: "claude"}, []Member{{TabID: tabB, Label: "reviewer", Vendor: "claude"}})

	msg, err := store.PostSystemMessage(g.ThreadID, "4341002a (claude) joined as a collaborator.")
	if err != nil {
		t.Fatalf("PostSystemMessage: %v", err)
	}
	if msg.TabID != SystemAuthor {
		t.Errorf("author = %q, want %q", msg.TabID, SystemAuthor)
	}
	for _, tab := range []string{tabA, tabB} {
		pending, err := store.PendingForTab(tab)
		if err != nil || len(pending) != 1 || pending[0].All.Count != 1 {
			t.Errorf("PendingForTab(%s) = %+v, %v, want one message for everyone", tab, pending, err)
		}
	}
	if labels := store.Labels([]string{SystemAuthor}); labels[SystemAuthor] != SystemAuthor {
		t.Errorf("system author label = %q, want %q", labels[SystemAuthor], SystemAuthor)
	}
}

func TestSystemMessageRefusesAdHocAndDisbandedThreads(t *testing.T) {
	store := openTestStore(t)
	store.RegisterTab(tabA, "claude")
	adHoc, _ := store.CreateThread(tabA, "title", "hello")
	if _, err := store.PostSystemMessage(adHoc, "hi"); err == nil {
		t.Error("system message accepted in an ad hoc thread")
	}
	g, _ := store.CreateGroup("g", Member{TabID: tabB, Label: "lead", Vendor: "claude"}, nil)
	store.DisbandGroup(g.ID)
	if _, err := store.PostSystemMessage(g.ThreadID, "hi"); err == nil {
		t.Error("system message accepted in a disbanded group")
	}
}
