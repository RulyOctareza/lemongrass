package hook

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func memoryGateFixture(t *testing.T) (home, projectPath string) {
	t.Helper()
	home = t.TempDir()
	t.Setenv("HOME", home)
	projectPath = t.TempDir()
	if err := os.MkdirAll(filepath.Join(projectPath, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}
	return home, projectPath
}

func TestTouchesClaudeMemoryAcrossRoutes(t *testing.T) {
	home, projectPath := memoryGateFixture(t)
	memDir, _ := claudeMemoryDir(projectPath)
	otherMem := filepath.Join(home, ".claude", "projects", "-some-other-project", "memory", "x.md")
	inMem := filepath.Join(memDir, "feedback_x.md")

	cases := []struct {
		name  string
		tool  string
		input string
		cwd   string
		want  bool
	}{
		{"write absolute", "Write", `{"file_path":"` + inMem + `"}`, "", true},
		{"edit other project memory", "Edit", `{"file_path":"` + otherMem + `"}`, "", true},
		{"write tilde", "Write", `{"file_path":"~/.claude/projects/-x/memory/a.md"}`, "", true},
		{"write account tilde", "Write", `{"file_path":"~/.claude-work/projects/-x/memory/a.md"}`, "", true},
		{"write account absolute", "Write", `{"file_path":"` + filepath.Join(home, ".claude-work", "projects", "-x", "memory", "a.md") + `"}`, "", true},
		{"bash tee account", "Bash", `{"command":"printf x | tee ~/.claude-work/projects/-x/memory/a.md"}`, "", true},
		{"write account skills", "Write", `{"file_path":"~/.claude-work/skills/a/SKILL.md"}`, "", false},
		{"write similar dir", "Write", `{"file_path":"~/.claudex/projects/-x/memory/a.md"}`, "", false},
		{"write dotdot", "Write", `{"file_path":"` + filepath.Join(memDir, "..", "memory", "a.md") + `"}`, "", true},
		{"write relative from cwd", "Write", `{"file_path":"a.md"}`, memDir, true},
		{"multiedit", "MultiEdit", `{"file_path":"` + inMem + `"}`, "", true},
		{"notebook", "NotebookEdit", `{"notebook_path":"` + inMem + `"}`, "", true},
		{"patch", "apply_patch", `{"command":"*** Add File: ` + inMem + `\n"}`, "", true},
		{"bash redirect", "Bash", `{"command":"echo hi > ` + inMem + `"}`, "", true},
		{"bash tee tilde", "Bash", `{"command":"printf x | tee ~/.claude/projects/-x/memory/a.md"}`, "", true},
		{"bash sed in place", "Bash", `{"command":"sed -i s/a/b/ $HOME/.claude/projects/-x/memory/a.md"}`, "", true},
		{"bash python", "Bash", `{"command":"python3 -c 'open(\"/home/u/.claude/projects/-x/memory/a.md\",\"w\")'"}`, "", true},
		{"bash write relative from memory cwd", "Bash", `{"command":"cat > a.md"}`, memDir, true},
		{"bash read only", "Bash", `{"command":"cat ` + inMem + `"}`, "", false},
		{"bash read with devnull", "Bash", `{"command":"ls ` + memDir + ` 2>/dev/null"}`, "", false},
		{"write outside memory", "Write", `{"file_path":"` + filepath.Join(projectPath, "main.go") + `"}`, "", false},
		{"bash unrelated write", "Bash", `{"command":"echo hi > /tmp/a.txt"}`, "", false},
		{"bash write elsewhere then grep memory", "Bash", `{"command":"printf '%s\\n' x >> biblio/scratchpad/t/activities.md && mv biblio/scratchpad/t biblio/scratchpad/archive/ && grep -rln t biblio --include=*.md --exclude-dir=archive; grep -rn t ` + filepath.Join(home, ".claude", "projects", "-p", "memory") + ` 2>/dev/null | head -3; ls biblio/scratchpad/archive | grep t"}`, "", false},
		{"bash grep memory piped with write elsewhere", "Bash", `{"command":"grep x ` + memDir + ` 2>&1 | head; echo hi > /tmp/a.txt"}`, "", false},
		{"bash find memory then write elsewhere", "Bash", `{"command":"find ` + memDir + ` -name '*.md' && touch /tmp/a"}`, "", false},
		{"bash cd memory then write", "Bash", `{"command":"cd ` + memDir + ` && echo hi > a.md"}`, "", true},
		{"bash loop over memory", "Bash", `{"command":"for f in ` + memDir + `*; do rm $f; done"}`, "", true},
		{"bash variable holding memory", "Bash", `{"command":"M=` + memDir + `; echo hi > $M/a.md"}`, "", true},
		{"bash grep memory redirected", "Bash", `{"command":"grep x ` + memDir + ` > /tmp/out"}`, "", true},
		{"bash find memory delete", "Bash", `{"command":"find ` + memDir + ` -name '*.md' -delete"}`, "", true},
		{"bash find memory exec", "Bash", `{"command":"find ` + memDir + ` -exec cp /tmp/a {} \\;"}`, "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := hookEvent{ToolName: tc.tool, ToolInput: json.RawMessage(tc.input), Cwd: tc.cwd}
			if got := touchesClaudeMemory(payload, projectPath); got != tc.want {
				t.Errorf("touchesClaudeMemory = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestMemoryFeedbackDenyCoversBashUntilSigned(t *testing.T) {
	store := openHookTestStore(t)
	_, projectPath := memoryGateFixture(t)
	memDir, _ := claudeMemoryDir(projectPath)
	payload := hookEvent{
		SessionID:         "session-a",
		ToolName:          "Bash",
		ToolInput:         json.RawMessage(`{"command":"echo x >> ` + filepath.Join(memDir, "feedback_x.md") + `"}`),
		EnforceBibliothek: true,
	}
	if result := hookPreToolUse(store, payload, projectPath); result.PermissionDecision != "deny" {
		t.Fatalf("PermissionDecision = %q, want deny for a Bash write into memory", result.PermissionDecision)
	}
	if err := store.Sign("session-a", memoryWriteWord); err != nil {
		t.Fatal(err)
	}
	if result := hookPreToolUse(store, payload, projectPath); result.PermissionDecision == "deny" {
		t.Errorf("PermissionDecision = %q, want no deny after signing", result.PermissionDecision)
	}
}
