// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package workgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validYAML = `
name: schema-review
leader_label: lead
thinkers:
  - label: reviewer
    vendor: claude
    model: sonnet
    prompt: |
      Review the schema changes.
  - label: tester
    vendor: codex
    prompt: Write the tests.
`

func TestParseYAMLAppliesFields(t *testing.T) {
	cfg, err := Parse([]byte(validYAML), t.TempDir())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Name != "schema-review" || cfg.LeaderLabel != "lead" || len(cfg.Thinkers) != 2 {
		t.Fatalf("cfg = %+v", cfg)
	}
	if cfg.Thinkers[0].Prompt != "Review the schema changes." || cfg.Thinkers[0].Model != "sonnet" || cfg.Thinkers[1].Vendor != "codex" {
		t.Errorf("thinkers = %+v", cfg.Thinkers)
	}
}

func TestParseAcceptsJSONAndDefaults(t *testing.T) {
	cfg, err := Parse([]byte(`{"name":"audit","thinkers":[{"label":"a","vendor":"claude","prompt":"go"}]}`), t.TempDir())
	if err != nil {
		t.Fatalf("Parse JSON: %v", err)
	}
	if cfg.Name != "audit" || cfg.LeaderLabel != "leader" {
		t.Errorf("parsed = %q %q, want audit leader", cfg.Name, cfg.LeaderLabel)
	}
}

func TestParseRequiresAName(t *testing.T) {
	for _, body := range []string{
		`thinkers: [{label: a, vendor: claude, prompt: go}]`,
		`{"name":"   ","thinkers":[{"label":"a","vendor":"claude","prompt":"go"}]}`,
	} {
		_, err := Parse([]byte(body), t.TempDir())
		if err == nil || !strings.HasPrefix(err.Error(), "workgroup: ") || !strings.Contains(err.Error(), "needs a name") {
			t.Errorf("Parse(%q) error = %v, want a workgroup: error asking for a name", body, err)
		}
	}
}

func TestParseReadsPromptFileRelativeToTheConfig(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "prompts"), 0o755)
	os.WriteFile(filepath.Join(dir, "prompts", "a.md"), []byte("from a file\n"), 0o600)
	path := filepath.Join(dir, "team.yaml")
	os.WriteFile(path, []byte("name: t\nthinkers:\n  - {label: a, vendor: codex, prompt_file: prompts/a.md}\n"), 0o600)

	cfg, err := Load(path)
	if err != nil || cfg.Thinkers[0].Prompt != "from a file" || cfg.Thinkers[0].PromptFile != "" {
		t.Fatalf("Load = %+v, %v, want the file's text as the prompt", cfg, err)
	}
}

func TestParseRejectsInvalidConfigs(t *testing.T) {
	long := strings.Repeat("x", MaxPromptRunes+1)
	cases := map[string]string{
		"no thinkers":        "name: t\nthinkers: []",
		"unknown key":        "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, extra: 1}",
		"bad vendor":         "name: t\nthinkers:\n  - {label: a, vendor: cursor, prompt: x}",
		"bad label":          "name: t\nthinkers:\n  - {label: 'has space', vendor: claude, prompt: x}",
		"duplicate label":    "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x}\n  - {label: A, vendor: codex, prompt: y}",
		"leader label clash": "name: t\nleader_label: a\nthinkers:\n  - {label: a, vendor: claude, prompt: x}",
		"bad model":          "name: t\nthinkers:\n  - {label: a, vendor: claude, model: 'x y', prompt: x}",
		"both prompts":       "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, prompt_file: y.md}",
		"no prompt":          "name: t\nthinkers:\n  - {label: a, vendor: claude}",
		"missing file":       "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt_file: nope.md}",
		"prompt too long":    "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: " + long + "}",
		"not yaml":           "name: t\nthinkers: [unclosed",
	}
	for name, body := range cases {
		if _, err := Parse([]byte(body), t.TempDir()); err == nil {
			t.Errorf("%s accepted", name)
		} else if !strings.HasPrefix(err.Error(), "workgroup: ") {
			t.Errorf("%s error %q lacks the package prefix", name, err)
		}
	}
}

func TestSkillsAreDedupedAndTheThinkerSkillIsImplicit(t *testing.T) {
	cfg, err := Parse([]byte("name: t\nthinkers:\n  - {label: a, vendor: codex, prompt: x, skills: [lgrass-connector, lgrass-howtobe-thinker, bibliothek, lgrass-connector]}"), t.TempDir())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	c := cfg.Thinkers[0]
	if got := c.RequiredSkills(); len(got) != 3 || got[0] != "lgrass-howtobe-thinker" || got[1] != "lgrass-connector" || got[2] != "bibliothek" {
		t.Errorf("RequiredSkills = %v, want the thinker skill first then the listed ones once each", got)
	}
	claude, _ := Parse([]byte("name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, skills: [bibliothek, lgrass-howtodo-work]}"), t.TempDir())
	if got := claude.Thinkers[0].RequiredSkills(); len(got) != 2 || got[1] != "lgrass-howtodo-work" {
		t.Errorf("RequiredSkills of a Claude thinker = %v, want bibliothek left out since its prompt carries it", got)
	}
	plain, _ := Parse([]byte("name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x}"), t.TempDir())
	if got := plain.Thinkers[0].RequiredSkills(); len(got) != 1 || got[0] != "lgrass-howtobe-thinker" {
		t.Errorf("RequiredSkills without a list = %v", got)
	}
}

func TestSkillNamesAreValidated(t *testing.T) {
	for name, body := range map[string]string{
		"path in name": "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, skills: ['../evil']}",
		"space":        "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, skills: ['a b']}",
		"too many":     "name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, skills: [a1, a2, a3, a4, a5, a6, a7, a8, a9]}",
	} {
		if _, err := Parse([]byte(body), t.TempDir()); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestCheckSkillsInstalledPerVendor(t *testing.T) {
	home := t.TempDir()
	install := func(vendorDir, name string) {
		dir := filepath.Join(home, vendorDir, "skills", name)
		os.MkdirAll(dir, 0o755)
		os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("x"), 0o600)
	}
	install(".claude", "lgrass-howtobe-thinker")
	install(".claude", "bibliothek")
	install(".codex", "lgrass-howtobe-thinker")

	cfg, _ := Parse([]byte("name: t\nthinkers:\n  - {label: a, vendor: claude, prompt: x, skills: [bibliothek]}\n  - {label: b, vendor: codex, prompt: y}"), t.TempDir())
	if err := cfg.CheckSkillsInstalled(home); err != nil {
		t.Fatalf("CheckSkillsInstalled: %v", err)
	}

	cfg, _ = Parse([]byte("name: t\nthinkers:\n  - {label: b, vendor: codex, prompt: y, skills: [bibliothek]}"), t.TempDir())
	err := cfg.CheckSkillsInstalled(home)
	if err == nil || !strings.Contains(err.Error(), `"bibliothek"`) || !strings.Contains(err.Error(), "lgrass-howtobe-thinker") {
		t.Errorf("error = %v, want the missing codex skill named and the installed ones listed", err)
	}
	if err := (Config{Thinkers: []Thinker{{Label: "a", Vendor: "claude"}}}).CheckSkillsInstalled(t.TempDir()); err == nil {
		t.Error("a home with no skills passed the check")
	}
}

func TestParseAdditionTakesOnlyThinkersWithFreshLabels(t *testing.T) {
	taken := []string{"lead", "reviewer"}
	cfg, err := ParseAddition([]byte("thinkers:\n  - {label: tester, vendor: codex, prompt: Write tests.}\n  - {label: docs, vendor: claude, prompt: Write docs.}"), t.TempDir(), taken)
	if err != nil {
		t.Fatalf("ParseAddition: %v", err)
	}
	if len(cfg.Thinkers) != 2 || cfg.Thinkers[0].Label != "tester" || cfg.Name != "" {
		t.Errorf("cfg = %+v", cfg)
	}

	cases := map[string]string{
		"name given":      "name: other\nthinkers:\n  - {label: a, vendor: claude, prompt: x}",
		"leader given":    "leader_label: boss\nthinkers:\n  - {label: a, vendor: claude, prompt: x}",
		"no thinkers":     "thinkers: []",
		"label taken":     "thinkers:\n  - {label: Reviewer, vendor: claude, prompt: x}",
		"leader taken":    "thinkers:\n  - {label: lead, vendor: claude, prompt: x}",
		"duplicate label": "thinkers:\n  - {label: a, vendor: claude, prompt: x}\n  - {label: a, vendor: codex, prompt: y}",
	}
	for name, body := range cases {
		if _, err := ParseAddition([]byte(body), t.TempDir(), taken); err == nil {
			t.Errorf("%s accepted", name)
		} else if !strings.HasPrefix(err.Error(), "workgroup: ") {
			t.Errorf("%s error %q lacks the package prefix", name, err)
		}
	}
}

func TestParseHasNoUpperThinkerLimit(t *testing.T) {
	var body strings.Builder
	body.WriteString("name: big\nthinkers:\n")
	for i := 0; i < 12; i++ {
		fmt.Fprintf(&body, "  - {label: t%d, vendor: claude, prompt: x}\n", i)
	}
	cfg, err := Parse([]byte(body.String()), t.TempDir())
	if err != nil || len(cfg.Thinkers) != 12 {
		t.Errorf("Parse of 12 thinkers = %d, %v", len(cfg.Thinkers), err)
	}
}
