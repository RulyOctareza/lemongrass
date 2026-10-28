// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestArchivedTaskSlugs(t *testing.T) {
	cases := []struct {
		command string
		want    []string
	}{
		{"mv biblio/scratchpad/fix-x biblio/scratchpad/archive/", []string{"fix-x"}},
		{"mv biblio/scratchpad/fix-x/ biblio/scratchpad/archive", []string{"fix-x"}},
		{`mv "biblio/scratchpad/a" biblio/scratchpad/b /p/biblio/scratchpad/archive/`, []string{"a", "b"}},
		{"cd /p && git mv -v biblio/scratchpad/fix-x biblio/scratchpad/archive/ && ls", []string{"fix-x"}},
		{"mv biblio/handover/x.md biblio/handover/archive/", nil},
		{"mv biblio/scratchpad/a biblio/scratchpad/b", nil},
		{"mv biblio/scratchpad/archive/old biblio/scratchpad/archive/", nil},
		{"cat biblio/scratchpad/archive/x/prd.md", nil},
	}
	for _, tc := range cases {
		if got := archivedTaskSlugs(tc.command); !reflect.DeepEqual(got, tc.want) {
			t.Errorf("archivedTaskSlugs(%q) = %v, want %v", tc.command, got, tc.want)
		}
	}
}

func TestCloseGateDeniesArchiveUntilHowRuns(t *testing.T) {
	store := openHookTestStore(t)
	projectPath := t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectPath, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}
	bash := func(tab, command string) hookEvent {
		input, _ := json.Marshal(map[string]string{"command": command})
		return hookEvent{SessionID: "s", TabID: tab, ToolName: "Bash", ToolInput: input, Cwd: projectPath}
	}
	archive := "mv biblio/scratchpad/fix-x biblio/scratchpad/archive/"

	result := hookPreToolUse(store, bash("tab-a", archive), projectPath)
	if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "lgrass how task close fix-x") {
		t.Fatalf("archive before how = %q %v, want deny naming the command", result.PermissionDecision, result.PermissionDecisionReason)
	}

	hookPreToolUse(store, bash("tab-a", "lgrass how task close biblio/scratchpad/fix-x/"), projectPath)
	if result := hookPreToolUse(store, bash("tab-a", archive), projectPath); result.PermissionDecision == "deny" {
		t.Fatalf("archive after how denied: %v", result.PermissionDecisionReason)
	}
	if result := hookPreToolUse(store, bash("tab-b", archive), projectPath); result.PermissionDecision != "deny" {
		t.Fatal("another tab inherited the mark")
	}
	if result := hookPreToolUse(store, bash("", archive), projectPath); result.PermissionDecision == "deny" {
		t.Fatal("a call with no tab id was gated")
	}
	both := "lgrass how task close other && mv biblio/scratchpad/other biblio/scratchpad/archive/"
	if result := hookPreToolUse(store, bash("tab-b", both), projectPath); result.PermissionDecision == "deny" {
		t.Fatalf("how and archive in one call denied: %v", result.PermissionDecisionReason)
	}
}
