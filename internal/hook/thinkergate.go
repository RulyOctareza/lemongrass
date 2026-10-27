// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/faizalv/lemongrass/internal/session"
)

// Tool names an agent uses for a shell command. Claude reports Bash, and the Codex names are the ones its shell tools are known by.
var shellToolNames = map[string]bool{"Bash": true, "shell": true, "local_shell": true, "exec_command": true, "container.exec": true}

// A command holding any of these can chain or redirect, so it is never treated as a plain lgrass or read command.
const shellMetacharacters = ";&|<>`$\n\r"

// The commands an agent may run to read a skill's SKILL.md on vendors with no skill tool.
var skillReaders = map[string]bool{"cat": true, "sed": true, "head": true, "tail": true, "nl": true, "less": true, "more": true, "bat": true}

type thinkerGateInput struct {
	ToolName  string
	ToolInput json.RawMessage
	Required  []string
	Marks     map[string]bool
}

// Returns the deny message, "" to allow, and the required skill this call loads, "" if none. The caller records that skill's mark.
func thinkerGate(in thinkerGateInput) (deny, loaded string) {
	loaded = requiredSkillLoad(in)

	var missing []string
	for _, name := range in.Required {
		if !in.Marks[session.SkillMark(name)] && name != loaded {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return "", loaded
	}
	if loaded != "" || isLgrassCommand(shellCommand(in.ToolName, in.ToolInput)) {
		return "", loaded
	}
	return session.FormatThinkerSkillsDeny(missing), ""
}

// The command string of a shell tool call, "" for any other tool. Codex passes an argv array, whose last element is the script after a -c style flag.
func shellCommand(toolName string, raw json.RawMessage) string {
	if !shellToolNames[toolName] {
		return ""
	}
	var input struct {
		Command json.RawMessage `json:"command"`
		Cmd     string          `json:"cmd"`
	}
	if json.Unmarshal(raw, &input) != nil {
		return ""
	}
	if input.Cmd != "" {
		return input.Cmd
	}
	var text string
	if json.Unmarshal(input.Command, &text) == nil {
		return text
	}
	var argv []string
	if json.Unmarshal(input.Command, &argv) != nil || len(argv) == 0 {
		return ""
	}
	if n := len(argv); n >= 3 && strings.HasSuffix(argv[n-2], "c") && strings.HasPrefix(argv[n-2], "-") {
		return argv[n-1]
	}
	return strings.Join(argv, " ")
}

func plainCommand(command string) []string {
	command = strings.TrimSpace(command)
	if command == "" || strings.ContainsAny(command, shellMetacharacters) {
		return nil
	}
	return strings.Fields(command)
}

// A single command whose program is exactly lgrass, with nothing that could chain or redirect. An absolute path is refused, since any file can be named lgrass.
func isLgrassCommand(command string) bool {
	fields := plainCommand(command)
	return len(fields) > 0 && fields[0] == "lgrass"
}

func requiredSkillLoad(in thinkerGateInput) string {
	if in.ToolName == "Skill" {
		var input struct {
			Skill string `json:"skill"`
		}
		if json.Unmarshal(in.ToolInput, &input) == nil {
			for _, name := range in.Required {
				if input.Skill == name {
					return name
				}
			}
		}
		return ""
	}
	fields := plainCommand(shellCommand(in.ToolName, in.ToolInput))
	if len(fields) < 2 || !skillReaders[fields[0]] {
		return ""
	}
	for _, name := range in.Required {
		pattern := regexp.MustCompile(`(^|/)skills/` + regexp.QuoteMeta(name) + `/SKILL\.md$`)
		for _, field := range fields[1:] {
			if pattern.MatchString(strings.Trim(field, `"'`)) {
				return name
			}
		}
	}
	return ""
}
