// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

const gateTabCollab = "cccccccc-7777-4777-8777-777777777777"

func TestCollaboratorIsHeldToLgrassUntilItReadsItsRole(t *testing.T) {
	store, group := storeWithGroup(t, nil)
	if err := store.AddCollaborator(group.ID, session.Member{TabID: gateTabCollab, Label: "research", Vendor: "claude"}); err != nil {
		t.Fatalf("AddCollaborator: %v", err)
	}
	dir := t.TempDir()

	denied := hookPreToolUse(store, preTool(gateTabCollab, "Read", map[string]string{"file_path": "/etc/hostname"}), dir)
	if denied.PermissionDecision != "deny" || !strings.Contains(denied.PermissionDecisionReason, session.CollaboratorJoinCommand) {
		t.Fatalf("a fresh collaborator was not held: %+v", denied)
	}
	if r := hookPreToolUse(store, preTool(gateTabCollab, "Bash", map[string]string{"command": "lgrass workgroup thread"}), dir); r.PermissionDecision == "deny" {
		t.Errorf("an lgrass command was denied: %s", r.PermissionDecisionReason)
	}
	if r := hookPreToolUse(store, preTool(gateTabCollab, "Bash", map[string]string{"command": "lgrass how collaborator join"}), dir); r.PermissionDecision == "deny" {
		t.Fatalf("the role read was denied: %s", r.PermissionDecisionReason)
	}
	if r := hookPreToolUse(store, preTool(gateTabCollab, "Read", map[string]string{"file_path": "/etc/hostname"}), dir); r.PermissionDecision == "deny" {
		t.Errorf("denied after reading the role: %s", r.PermissionDecisionReason)
	}

	start := hookSessionStart(store, hookEvent{SessionID: "s9", TabID: gateTabCollab}, dir)
	if !strings.Contains(start.AdditionalContext, `collaborator "research"`) || !strings.Contains(start.AdditionalContext, session.CollaboratorJoinCommand) {
		t.Errorf("start context = %q", start.AdditionalContext)
	}
	if r := hookPreToolUse(store, preTool(gateTabCollab, "Read", map[string]string{"file_path": "/etc/hostname"}), dir); r.PermissionDecision != "deny" {
		t.Error("the mark survived a session start")
	}
}

func TestThinkersAndPlainTabsAreNotHeldByTheCollaboratorGate(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	store.MarkReady(gateTabClaude, session.SkillMark("lgrass-howtobe-thinker"))
	for _, tab := range []string{gateTabClaude, gateTabLeader, gateTabOutside} {
		if deny := collaboratorGateDeny(store, preTool(tab, "Read", map[string]string{"file_path": "/etc/hostname"})); deny != "" {
			t.Errorf("tab %s held by the collaborator gate: %s", tab, deny)
		}
	}
}
