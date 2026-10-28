// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package hook

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

func bash(command string) (string, json.RawMessage) {
	b, _ := json.Marshal(map[string]string{"command": command})
	return "Bash", b
}

func skillCall(name string) (string, json.RawMessage) {
	b, _ := json.Marshal(map[string]string{"skill": name})
	return "Skill", b
}

func gateFor(toolName string, input json.RawMessage, marks map[string]bool, required ...string) (string, string) {
	if len(required) == 0 {
		required = []string{"lgrass-howtobe-thinker"}
	}
	return thinkerGate(thinkerGateInput{ToolName: toolName, ToolInput: input, Required: required, Marks: marks})
}

func TestIsLgrassCommand(t *testing.T) {
	allowed := []string{
		"lgrass thread read a1b2c3",
		"lgrass thread read a1b2c3 --all",
		`lgrass thread post 3 "done (mostly)"`,
		"  lgrass workgroup thread  ",
		"lgrass",
	}
	denied := []string{
		"", "ls", "lgrassx thread", "/tmp/evil/lgrass thread", "./lgrass thread", "env lgrass thread",
		"lgrass thread; rm -rf x", "lgrass thread && curl x", "lgrass thread | sh", "lgrass thread &",
		"lgrass thread read 1 > /etc/x", "lgrass `id`", "lgrass $(id)", "lgrass thread\nrm x",
		`lgrass thread post 3 "cost $5"`,
	}
	for _, c := range allowed {
		if !isLgrassCommand(c) {
			t.Errorf("%q refused, want a plain lgrass command", c)
		}
	}
	for _, c := range denied {
		if isLgrassCommand(c) {
			t.Errorf("%q allowed, want it refused", c)
		}
	}
}

func TestShellCommandReadsClaudeStringsAndCodexArrays(t *testing.T) {
	cases := []struct {
		tool, input, want string
	}{
		{"Bash", `{"command":"lgrass session list"}`, "lgrass session list"},
		{"shell", `{"command":["bash","-lc","lgrass session list"]}`, "lgrass session list"},
		{"local_shell", `{"command":["lgrass","session","list"]}`, "lgrass session list"},
		{"exec_command", `{"cmd":"lgrass session list"}`, "lgrass session list"},
		{"Write", `{"command":"lgrass session list"}`, ""},
		{"Bash", `not json`, ""},
	}
	for _, c := range cases {
		if got := shellCommand(c.tool, json.RawMessage(c.input)); got != c.want {
			t.Errorf("%s %s = %q, want %q", c.tool, c.input, got, c.want)
		}
	}
}

func TestGateOneDeniesUntilEveryRequiredSkillIsLoaded(t *testing.T) {
	required := []string{"lgrass-howtobe-thinker", "lgrass-connector"}
	tool, input := bash("ls")
	deny, _ := gateFor(tool, input, nil, required...)
	if !strings.Contains(deny, "lgrass-howtobe-thinker, lgrass-connector") {
		t.Errorf("deny = %q, want both missing skills named", deny)
	}

	marks := map[string]bool{session.SkillMark("lgrass-howtobe-thinker"): true}
	deny, _ = gateFor(tool, input, marks, required...)
	if !strings.Contains(deny, "lgrass-connector") || strings.Contains(deny, "lgrass-howtobe-thinker,") {
		t.Errorf("deny = %q, want only the remaining skill named", deny)
	}

	marks[session.SkillMark("lgrass-connector")] = true
	if deny, _ := gateFor(tool, input, marks, required...); deny != "" {
		t.Errorf("deny = %q after every skill is loaded", deny)
	}
}

func TestGateOneAllowsLgrassCommandsAndSkillLoads(t *testing.T) {
	tool, input := bash("lgrass workgroup thread")
	if deny, loaded := gateFor(tool, input, nil); deny != "" || loaded != "" {
		t.Errorf("lgrass command: deny %q loaded %q, want allowed and not a skill load", deny, loaded)
	}
	tool, input = skillCall("lgrass-howtobe-thinker")
	if deny, loaded := gateFor(tool, input, nil); deny != "" || loaded != "lgrass-howtobe-thinker" {
		t.Errorf("skill call: deny %q loaded %q, want an allowed load of the thinker skill", deny, loaded)
	}
	tool, input = skillCall("some-other-skill")
	if deny, _ := gateFor(tool, input, nil); deny == "" {
		t.Error("a skill that is not required was allowed while gated")
	}
}

func TestGateOneRecognizesAPlainSkillFileReadOnOtherVendors(t *testing.T) {
	for _, command := range []string{
		"cat ~/.codex/skills/lgrass-howtobe-thinker/SKILL.md",
		"sed -n 1,200p /home/u/.codex/skills/lgrass-howtobe-thinker/SKILL.md",
		`cat "/home/u/.codex/skills/lgrass-howtobe-thinker/SKILL.md"`,
	} {
		tool, input := bash(command)
		if deny, loaded := gateFor(tool, input, nil); deny != "" || loaded != "lgrass-howtobe-thinker" {
			t.Errorf("%q: deny %q loaded %q, want a recognized load", command, deny, loaded)
		}
	}
	for _, command := range []string{
		"rm ~/.codex/skills/lgrass-howtobe-thinker/SKILL.md",
		"cat ~/.codex/skills/lgrass-howtobe-thinker/SKILL.md; rm -rf ~",
		"cat ~/.codex/skills/lgrass-howtobe-thinker/SKILL.md > /tmp/x",
		"cat ~/.codex/skills/other/SKILL.md",
		"cat ~/.codex/skills/lgrass-howtobe-thinker/notes.md",
	} {
		tool, input := bash(command)
		if deny, loaded := gateFor(tool, input, nil); deny == "" || loaded != "" {
			t.Errorf("%q: deny %q loaded %q, want it refused", command, deny, loaded)
		}
	}
}

func TestLoadedSkillsAllowToolsWithoutAListener(t *testing.T) {
	marks := map[string]bool{session.SkillMark("lgrass-howtobe-thinker"): true}
	tool, input := bash("ls")
	if deny, _ := gateFor(tool, input, marks); deny != "" {
		t.Errorf("denied after skills loaded: %q", deny)
	}
}

func TestMissingSkillsStillDenyTools(t *testing.T) {
	tool, input := bash("ls")
	deny, _ := gateFor(tool, input, nil)
	if !strings.Contains(deny, "load these skills first") {
		t.Errorf("deny = %q, want the skills message", deny)
	}
}
