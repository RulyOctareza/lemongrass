// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"strings"
	"testing"
)

func TestManualsRender(t *testing.T) {
	root := t.TempDir()
	cases := map[string][]string{
		"ask request":   {"lgrass ask <path>", "type: db", "type: rester", "type: command", "lgrass ask show <id>", "Do not ask again with a variation"},
		"book write":    {"Split.", "never how it broke", "toc.md"},
		"git resolve":   {"lgrass git conflict show", "lgrass git conflict note", "lgrass git conflict submit", "never stage"},
		"laws write":    {"explicit yes", "biblio/laws/summary.md", "Never write a law to memory"},
		"leader create": {"lgrass workgroup create <config>", "lgrass how thinker start"},
		"thinker start": {"lgrass workgroup thread", "wait for the leader's go"},
	}
	for cmd, wants := range cases {
		out := render(t, root, strings.Fields(cmd)...)
		for _, want := range wants {
			if !strings.Contains(out, want) {
				t.Errorf("%s missing %q", cmd, want)
			}
		}
		if _, err := Render(root, append(strings.Fields(cmd), "extra")); err == nil {
			t.Errorf("%s accepted an argument", cmd)
		}
	}
}

func TestHandoverNeedsPRD(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/idea/notes.md", "loose")
	if out := render(t, root, "task", "handover", "idea"); !strings.Contains(out, "write the PRD first") {
		t.Fatalf("out = %q", out)
	}
}

func TestHandoverRefusesFinishedWork(t *testing.T) {
	root := t.TempDir()
	task(t, root, "done", "- [x] a", "- [x] b")
	if out := render(t, root, "task", "handover", "done"); !strings.Contains(out, "never written for finished work") {
		t.Fatalf("out = %q", out)
	}
}

func TestHandoverNew(t *testing.T) {
	root := t.TempDir()
	task(t, root, "feat", "- [x] a", "- [ ] b")
	out := render(t, root, "task", "handover", "feat")
	for _, want := range []string{"New handover for feat: 1 of 2", "biblio/handover/feat.md", "-> feat.md", "i-write-memory"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
}

func TestHandoverUpdateChecksWhiteboard(t *testing.T) {
	root := t.TempDir()
	task(t, root, "feat", "- [ ] b")
	writeFile(t, root, "biblio/handover/feat.md", "# feat")
	out := render(t, root, "task", "handover", "feat")
	if !strings.Contains(out, "Rewrite the Status") || !strings.Contains(out, "has no row") {
		t.Fatalf("out = %q", out)
	}
	writeFile(t, root, "biblio/handover/whiteboard.md", "1. feat [x] -> feat.md\n")
	if out := render(t, root, "task", "handover", "feat"); !strings.Contains(out, "stays") {
		t.Fatalf("with row out = %q", out)
	}
}
