// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestWindowsClaudeMemoryDirSlug(t *testing.T) {
	home := t.TempDir()
	t.Setenv("USERPROFILE", home)
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	dir, err := claudeMemoryDir(`C:\Users\ann\proj`)
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(home, ".claude", "projects", "C--Users-ann-proj", "memory") + `\`; dir != want {
		t.Errorf("claudeMemoryDir = %s, want %s", dir, want)
	}
}

func TestWindowsClaudeMemoryPathSpellings(t *testing.T) {
	home := t.TempDir()
	volume := filepath.VolumeName(home)
	gitBash := "/" + strings.ToLower(volume[:1]) + filepath.ToSlash(home[len(volume):])
	for _, p := range []string{
		home + `\.claude\projects\C--p\memory\a.md`,
		home + `\.Claude\Projects\C--p\Memory\a.md`,
		gitBash + "/.claude/projects/C--p/memory/a.md",
		`~\.claude-work\projects\C--p\memory\a.md`,
	} {
		if !isClaudeMemoryPath(p, "", home, "") {
			t.Errorf("%s is not seen as a memory path", p)
		}
	}
}

func TestWindowsShellTouchesClaudeMemory(t *testing.T) {
	home, projectPath := memoryGateFixture(t)
	t.Setenv("USERPROFILE", home)
	file := home + `\.Claude\Projects\C--p\memory\a.md`
	cases := map[string]bool{
		"echo x > '" + file + "'":      true,
		`echo x > "` + file + `"`:      true,
		"cp a.md '" + file + "'":       true,
		"cat '" + file + "'":           false,
		"cat '" + file + "' 2> nul":    false,
		"echo x > '" + home + `\a.md'`: false,
	}
	for command, want := range cases {
		input, _ := json.Marshal(map[string]string{"command": command})
		payload := hookEvent{ToolName: "Bash", ToolInput: input, Cwd: t.TempDir()}
		if got := touchesClaudeMemory(payload, projectPath); got != want {
			t.Errorf("%s: touches memory = %v, want %v", command, got, want)
		}
	}
}
