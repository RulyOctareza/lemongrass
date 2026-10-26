// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

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

func TestReconcileCodexHooksRegistersAllEvents(t *testing.T) {
	out, changed, err := reconcileCodexHooks([]byte(`{"theme":"dark"}`), testLgrass)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	if !strings.Contains(string(out), `"theme": "dark"`) {
		t.Error("unrelated Codex configuration was not preserved")
	}
	for _, event := range codexHookEvents {
		groups := groupsFor(t, out, event)
		if len(groups) != 1 {
			t.Fatalf("%s: %d groups, want 1", event, len(groups))
		}
		command, _ := groups[0]["hooks"].([]any)
		if !strings.Contains(string(out), "LGRASS_HOOK_VENDOR=codex "+testLgrass+" hook "+event) {
			t.Errorf("%s: Codex command missing", event)
		}
		if len(command) != 1 {
			t.Errorf("%s: handler count = %d, want 1", event, len(command))
		}
	}
	start := groupsFor(t, out, "SessionStart")[0]
	if start["matcher"] != "startup|resume|clear|compact" {
		t.Errorf("SessionStart matcher = %v", start["matcher"])
	}
	if startHooks, _ := start["hooks"].([]any); len(startHooks) != 1 {
		t.Errorf("SessionStart handlers = %d, want 1", len(startHooks))
	}
	end := groupsFor(t, out, "SessionEnd")[0]
	if endHooks, _ := end["hooks"].([]any); len(endHooks) != 1 {
		t.Errorf("SessionEnd handlers = %d, want 1", len(endHooks))
	}

	second, changed, err := reconcileCodexHooks(out, testLgrass)
	if err != nil || changed || string(second) != string(out) {
		t.Fatalf("second pass changed=%v err=%v", changed, err)
	}
}

func TestReconcileCodexLifecycleHooksPreservesForeignHandlers(t *testing.T) {
	in := []byte(`{"hooks":{"Stop":[{"hooks":[{"type":"command","command":"other-tool stop"}]}],"PermissionRequest":[{"matcher":"Bash","hooks":[{"type":"command","command":"other-tool approval"}]}]}}`)
	out, changed, err := reconcileCodexHooks(in, testLgrass)
	if err != nil || !changed {
		t.Fatalf("changed=%v err=%v", changed, err)
	}
	for _, event := range []string{"UserPromptSubmit", "Stop", "PermissionRequest", "Interrupt"} {
		groups := groupsFor(t, out, event)
		owned := 0
		for _, group := range groups {
			handlers, _ := group["hooks"].([]any)
			for _, raw := range handlers {
				handler, _ := raw.(map[string]any)
				if handler["command"] != "LGRASS_HOOK_VENDOR=codex "+testLgrass+" hook "+event {
					continue
				}
				owned++
				if _, matched := group["matcher"]; matched {
					t.Errorf("%s owned group is narrowed by a matcher", event)
				}
				if event == "Interrupt" && handler["timeout"] != float64(3) {
					t.Errorf("Interrupt timeout = %v, want 3", handler["timeout"])
				}
			}
		}
		if owned != 1 {
			t.Errorf("%s has %d owned handlers, want one", event, owned)
		}
	}
	for _, command := range []string{"other-tool stop", "other-tool approval"} {
		if !strings.Contains(string(out), command) {
			t.Errorf("foreign handler %q was lost", command)
		}
	}
}
