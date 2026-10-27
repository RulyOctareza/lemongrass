// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

import (
	"encoding/json"
	"strings"
	"testing"
)

const testLgrass = "/home/u/.lemongrass/bin/lgrassd"

func groupsFor(t *testing.T, data []byte, event string) []map[string]any {
	t.Helper()
	var top struct {
		Hooks map[string][]map[string]any `json:"hooks"`
	}
	if err := json.Unmarshal(data, &top); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	return top.Hooks[event]
}

func TestReconcileEmptyInputRegistersAllEvents(t *testing.T) {
	for _, in := range []string{"", "{}", "  \n"} {
		out, changed, err := ReconcileClaudeSettings([]byte(in), testLgrass)
		if err != nil || !changed {
			t.Fatalf("input %q: changed=%v err=%v", in, changed, err)
		}
		for _, event := range ClaudeHookEvents {
			groups := groupsFor(t, out, event)
			if len(groups) != 1 {
				t.Fatalf("%s: %d groups, want 1", event, len(groups))
			}
			if _, has := groups[0]["matcher"]; has {
				t.Errorf("%s: matcher present, want none", event)
			}
			if !strings.Contains(string(out), testLgrass+" hook "+event) {
				t.Errorf("%s: command missing", event)
			}
		}
	}
}

func TestReconcileIsIdempotent(t *testing.T) {
	first, _, err := ReconcileClaudeSettings(nil, testLgrass)
	if err != nil {
		t.Fatal(err)
	}
	second, changed, err := ReconcileClaudeSettings(first, testLgrass)
	if err != nil || changed {
		t.Fatalf("second pass changed=%v err=%v", changed, err)
	}
	if string(second) != string(first) {
		t.Error("unchanged pass altered the bytes")
	}
}

func TestReconcileHealsNarrowMatcher(t *testing.T) {
	in := `{"hooks":{"PreToolUse":[{"matcher":"Write|Edit","hooks":[{"type":"command","command":"` + testLgrass + ` hook PreToolUse"}]}]}}`
	out, changed, err := ReconcileClaudeSettings([]byte(in), testLgrass)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	groups := groupsFor(t, out, "PreToolUse")
	if len(groups) != 1 {
		t.Fatalf("%d groups, want 1", len(groups))
	}
	if _, has := groups[0]["matcher"]; has {
		t.Error("matcher survived the heal")
	}
}

func TestReconcileHealsStaleCommandPath(t *testing.T) {
	in := `{"hooks":{"SessionStart":[{"hooks":[{"type":"command","command":"/usr/local/bin/lgrass hook SessionStart"}]}]}}`
	out, _, err := ReconcileClaudeSettings([]byte(in), testLgrass)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "/usr/local/bin/lgrass") {
		t.Error("stale path survived")
	}
}

func TestReconcilePreservesUnrelatedConfig(t *testing.T) {
	in := `{
  "editorMode": "normal",
  "permissions": {"deny": ["Read(~/.ssh/**)"]},
  "hooks": {
    "PreToolUse": [
      {"matcher": "Bash", "hooks": [{"type": "command", "command": "other-tool check", "timeout": 5}]}
    ],
    "Stop": [{"hooks": [{"type": "command", "command": "notify"}]}]
  }
}`
	out, _, err := ReconcileClaudeSettings([]byte(in), testLgrass)
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, want := range []string{`"editorMode": "normal"`, `Read(~/.ssh/**)`, `other-tool check`, `"timeout": 5`, `"Stop"`, `notify`} {
		if !strings.Contains(s, want) {
			t.Errorf("lost %q", want)
		}
	}
	if got := len(groupsFor(t, out, "PreToolUse")); got != 2 {
		t.Errorf("PreToolUse groups = %d, want 2 (foreign kept, lgrass added)", got)
	}
}

func TestReconcileCollapsesDuplicateOwnedGroups(t *testing.T) {
	dup := `{"hooks":[{"type":"command","command":"` + testLgrass + ` hook SessionEnd"}]}`
	in := `{"hooks":{"SessionEnd":[` + dup + `,` + dup + `]}}`
	out, changed, err := ReconcileClaudeSettings([]byte(in), testLgrass)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if got := len(groupsFor(t, out, "SessionEnd")); got != 1 {
		t.Errorf("SessionEnd groups = %d, want 1", got)
	}
}

func TestReconcileRejectsInvalidJSON(t *testing.T) {
	for _, in := range []string{`{"hooks":`, `[1,2]`, `{"hooks":"nope"}`} {
		if _, _, err := ReconcileClaudeSettings([]byte(in), testLgrass); err == nil {
			t.Errorf("input %q: expected an error", in)
		}
	}
}
