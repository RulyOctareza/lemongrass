// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"testing"
)

func TestWindowsShellToolCommand(t *testing.T) {
	cases := []struct {
		name    string
		tool    string
		vendor  string
		input   string
		command string
		denied  bool
		isShell bool
	}{
		{"claude bash is judged as is", "Bash", "claude", `{"command":"rm -rf x"}`, "rm -rf x", false, true},
		{"codex bash string runs powershell", "Bash", "codex", `{"command":"Remove-Item x"}`, "", true, true},
		{"codex bash string starting bash", "Bash", "codex", `{"command":"bash -lc 'ls'"}`, "bash -lc 'ls'", false, true},
		{"codex shell argv with bash", "shell", "codex", `{"command":["bash","-lc","ls -la"]}`, "ls -la", false, true},
		{"codex shell argv with git bash path", "shell", "codex", `{"command":["C:\\Program Files\\Git\\bin\\bash.exe","-c","ls"]}`, "ls", false, true},
		{"codex shell argv with powershell", "shell", "codex", `{"command":["powershell.exe","-Command","ls"]}`, "", true, true},
		{"wsl bash is another filesystem", "shell", "codex", `{"command":["C:\\Windows\\System32\\bash.exe","-c","ls"]}`, "", true, true},
		{"exec_command with a bash shell", "exec_command", "codex", `{"cmd":"ls","shell":"bash"}`, "ls", false, true},
		{"exec_command in the default shell", "exec_command", "codex", `{"cmd":"dir"}`, "", true, true},
		{"powershell tool", "PowerShell", "claude", `{"command":"Get-ChildItem"}`, "", true, true},
		{"unreadable input", "shell", "codex", `{"command":42}`, "", true, true},
		{"read tool is not a shell", "Read", "claude", `{"file_path":"x"}`, "", false, false},
	}
	for _, tc := range cases {
		payload := hookEvent{ToolName: tc.tool, Vendor: tc.vendor, ToolInput: json.RawMessage(tc.input)}
		command, verdict, isShell := shellToolCommand(payload)
		if command != tc.command || (verdict != nil) != tc.denied || isShell != tc.isShell {
			t.Errorf("%s: got %q, denied %v, shell %v", tc.name, command, verdict != nil, isShell)
		}
		if verdict != nil && verdict.RuleID != "opaque-shell" {
			t.Errorf("%s: denied by %s, want opaque-shell", tc.name, verdict.RuleID)
		}
	}
}
