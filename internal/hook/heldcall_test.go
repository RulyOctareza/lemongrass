// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

func heldPayload(t *testing.T, tool string, input any, cwd string) hookEvent {
	t.Helper()
	in, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(map[string]any{"session_id": "session-a", "cwd": cwd, "tool_name": tool, "tool_input": json.RawMessage(in)})
	if err != nil {
		t.Fatal(err)
	}
	payload, err := claudeHookAdapter{}.decode(raw)
	if err != nil {
		t.Fatal(err)
	}
	payload.Raw = raw
	return payload
}

func TestMemoryGateHoldsBashAndWrite(t *testing.T) {
	store := openHookTestStore(t)
	_, projectPath := memoryGateFixture(t)
	memDir, _ := claudeMemoryDir(projectPath)
	inMem := filepath.Join(memDir, "MEMORY.md")

	for _, p := range []hookEvent{
		heldPayload(t, "Bash", map[string]string{"command": "echo x >> " + inMem}, projectPath),
		heldPayload(t, "Write", map[string]string{"file_path": inMem, "content": "x"}, projectPath),
	} {
		result := hookPreToolUse(store, p, projectPath)
		if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "lgrass replay ") {
			t.Errorf("%s: decision %q reason %q, want a deny naming lgrass replay", p.ToolName, result.PermissionDecision, result.PermissionDecisionReason)
		}
	}
	calls, err := store.HeldCalls("session-a")
	if err != nil || len(calls) != 2 {
		t.Fatalf("HeldCalls = %d, %v, want 2", len(calls), err)
	}
	rc, err := DecodeHeldCall(calls[1])
	if err != nil || rc.Tool != "Write" || rc.FilePath != inMem || rc.Content != "x" {
		t.Errorf("DecodeHeldCall = %+v, %v, want the Write call back", rc, err)
	}
}

func TestMemoryGateDoesNotHoldEditOrReplay(t *testing.T) {
	store := openHookTestStore(t)
	_, projectPath := memoryGateFixture(t)
	memDir, _ := claudeMemoryDir(projectPath)
	inMem := filepath.Join(memDir, "MEMORY.md")

	edit := heldPayload(t, "Edit", map[string]string{"file_path": inMem, "old_string": "a", "new_string": "b"}, projectPath)
	if result := hookPreToolUse(store, edit, projectPath); result.PermissionDecisionReason != session.FormatMemoryFeedbackDeny() {
		t.Errorf("Edit reason = %q, want the plain deny", result.PermissionDecisionReason)
	}
	t.Setenv(replayEnv, "1")
	bash := heldPayload(t, "Bash", map[string]string{"command": "echo x >> " + inMem}, projectPath)
	if result := hookPreToolUse(store, bash, projectPath); result.PermissionDecisionReason != session.FormatMemoryFeedbackDeny() {
		t.Errorf("replay reason = %q, want the plain deny", result.PermissionDecisionReason)
	}
	if calls, _ := store.HeldCalls("session-a"); len(calls) != 0 {
		t.Errorf("HeldCalls = %d, want none", len(calls))
	}
}

func fakeHookBinary(t *testing.T, output string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "lgrassd")
	script := "#!/bin/sh\ncat >/dev/null\n[ \"$" + replayEnv + "\" = 1 ] || exit 3\nprintf '%s' '" + output + "'\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRecheckHeldCallPassesDenyReasonThrough(t *testing.T) {
	deny := fakeHookBinary(t, `{"hookSpecificOutput":{"hookEventName":"PreToolUse","permissionDecision":"deny","permissionDecisionReason":"lgrass: blocked."}}`)
	reason, err := RecheckHeldCall(session.HeldCall{ID: "abc", HookBinary: deny, Payload: []byte(`{}`)})
	if err != nil || reason != "lgrass: blocked." {
		t.Errorf("RecheckHeldCall = %q, %v, want the deny reason", reason, err)
	}
	allow := fakeHookBinary(t, "")
	reason, err = RecheckHeldCall(session.HeldCall{ID: "abc", HookBinary: allow, Payload: []byte(`{}`)})
	if err != nil || reason != "" {
		t.Errorf("RecheckHeldCall = %q, %v, want a pass", reason, err)
	}
}

func TestCollapseNonMemoryHeredocs(t *testing.T) {
	_, projectPath := memoryGateFixture(t)
	memDir, _ := claudeMemoryDir(projectPath)

	command := "cd /p/scratch; cat >> act.md <<'EOF'\nline one\nline two\nEOF\n" +
		"python3 - <<'EOF'\nprint(1)\nEOF\n" +
		"cd " + memDir + "; cat >> MEMORY.md <<'EOF'\n- pointer line\nEOF\n"
	got := collapseNonMemoryHeredocs(command, "/p", projectPath)

	if strings.Contains(got, "line one") || !strings.Contains(got, "<<'EOF'\n[2 lines not shown, written outside memory]\nEOF\npython3") {
		t.Errorf("scratchpad heredoc not collapsed:\n%s", got)
	}
	if !strings.Contains(got, "print(1)") {
		t.Errorf("interpreter heredoc collapsed, want it shown:\n%s", got)
	}
	if !strings.Contains(got, "- pointer line") {
		t.Errorf("heredoc after the memory cd collapsed, want it shown:\n%s", got)
	}
	if fromMem := collapseNonMemoryHeredocs(command, memDir, projectPath); fromMem != command {
		t.Errorf("command run from inside memory collapsed, want it unchanged")
	}
}
