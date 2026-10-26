// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package hook

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/guard"
	"github.com/faizalv/lemongrass/internal/project"
)

func bashPayload(t *testing.T, command string) hookEvent {
	t.Helper()
	input, err := json.Marshal(map[string]string{"command": command})
	if err != nil {
		t.Fatal(err)
	}
	return hookEvent{ToolName: "Bash", ToolInput: input, TabID: "tab", Cwd: t.TempDir()}
}

func TestDangerDecisionToolInputs(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	cases := []struct {
		name  string
		tool  string
		input string
		want  string
	}{
		{"read normal file", "Read", `{"file_path":"/tmp/a.txt"}`, ""},
		{"read ssh key", "Read", `{"file_path":"~/.ssh/id_rsa"}`, "secret-path"},
		{"grep in aws dir", "Grep", `{"pattern":"key","path":"~/.aws"}`, "secret-path"},
		{"grep pattern only", "Grep", `{"pattern":".lemongrass","path":"src"}`, ""},
		{"glob into gnupg", "Glob", `{"pattern":"~/.gnupg/*"}`, "secret-path"},
		{"write settings", "Write", `{"file_path":"~/.claude/settings.json"}`, "guard-config"},
		{"edit project settings", "Edit", `{"file_path":".claude/settings.local.json"}`, "guard-config"},
		{"read settings", "Read", `{"file_path":"~/.claude/settings.json"}`, ""},
		{"patch into lgrass binary", "apply_patch", `{"command":"*** Update File: ~/.local/bin/lgrass\n"}`, "guard-config"},
		{"bash rm", "Bash", `{"command":"rm -rf build"}`, "rm-force"},
		{"bash ls", "Bash", `{"command":"ls"}`, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := hookEvent{ToolName: tc.tool, ToolInput: json.RawMessage(tc.input), TabID: "tab"}
			verdict := dangerDecision(payload)
			got := ""
			if verdict != nil {
				got = verdict.RuleID
			}
			if got != tc.want {
				t.Errorf("rule = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDangerGateOnlyForLemongrassTabs(t *testing.T) {
	store := openHookTestStore(t)
	payload := bashPayload(t, "rm -rf build")
	payload.TabID = ""
	if result := hookPreToolUse(store, payload, t.TempDir()); result.PermissionDecision == "deny" {
		t.Errorf("a session outside a lemongrass tab was denied: %s", result.PermissionDecisionReason)
	}
	payload.TabID = "tab"
	result := hookPreToolUse(store, payload, t.TempDir())
	if result.PermissionDecision != "deny" {
		t.Errorf("a lemongrass tab was not denied for rm -rf")
	}
}

func TestDangerVerdictPrefersDenyOverApproval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	verdict := dangerDecision(bashPayload(t, "rm -rf build; git add ."))
	if verdict == nil || verdict.Mode != guard.ModeDeny {
		t.Errorf("verdict = %+v, want a deny", verdict)
	}
}

func TestDangerDecisionRmScopedToLaunchProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Register(root); err != nil {
		t.Fatal(err)
	}
	payload := bashPayload(t, "rm -rf build")
	payload.Cwd = root

	t.Setenv(claudeProjectDirEnv, "")
	if verdict := dangerDecision(payload); verdict == nil || verdict.RuleID != "rm-force" {
		t.Errorf("without a launch project, verdict = %+v, want rm-force", verdict)
	}

	t.Setenv(claudeProjectDirEnv, root)
	if verdict := dangerDecision(payload); verdict != nil {
		t.Errorf("rm inside the launch project was denied by %s", verdict.RuleID)
	}

	other := t.TempDir()
	if _, err := project.Register(other); err != nil {
		t.Fatal(err)
	}
	payload.Cwd = other
	if verdict := dangerDecision(payload); verdict == nil || verdict.RuleID != "rm-force" {
		t.Errorf("rm after moving into another project, verdict = %+v, want rm-force", verdict)
	}
}

const runChildEnv = "LGRASS_TEST_RUN_CHILD"

func TestRunGatesTabOutsideRegisteredProjects(t *testing.T) {
	if os.Getenv(runChildEnv) != "" {
		Run([]string{"PreToolUse"}, "tab")
		return
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	if _, err := project.Register(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	input, err := json.Marshal(map[string]any{"session_id": "s", "cwd": t.TempDir(), "tool_name": "Bash", "tool_input": map[string]string{"command": "rm -rf build"}})
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestRunGatesTabOutsideRegisteredProjects$")
	cmd.Env = append(os.Environ(), runChildEnv+"=1", "HOME="+home, "LGRASS_HOOK_VENDOR=")
	cmd.Stdin = bytes.NewReader(input)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("child run failed: %v", err)
	}
	if !strings.Contains(string(out), `"permissionDecision":"deny"`) || !strings.Contains(string(out), "rm-force") {
		t.Errorf("a tab whose cwd is outside every project was not denied: %s", out)
	}
}
