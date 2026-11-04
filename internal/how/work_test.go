// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"strings"
	"testing"
)

func wantAll(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestWorkTaskNew(t *testing.T) {
	out := render(t, t.TempDir(), "work", "task", "feat")
	wantAll(t, out, "New task feat.", "Books first.", "biblio/scratchpad/feat/prd.md", "State how the work is done", "lgrass how task close feat")
}

func TestWorkTaskResumesFromFirstOpenItem(t *testing.T) {
	root := t.TempDir()
	task(t, root, "feat", "- [x] one", "- [ ] two.", "- [ ] three")
	writeFile(t, root, "biblio/handover/whiteboard.md", "1. feat [x] -> feat.md\n")
	out := render(t, root, "work", "task", "feat")
	wantAll(t, out, "Resume task feat: 2 of 3", `"two"`, "biblio/handover/feat.md", "While you work", "**Books first.**", "before you judge, extend or build on a plan", "stop and tell the user what is stale")
	if strings.Contains(out, "Plan first") {
		t.Error("resume carries the new-PRD steps")
	}
}

func TestWorkTaskDraftsMissingPlans(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/feat/prd.md", "# T\n\n## Problem\n\nx\n")
	wantAll(t, render(t, root, "work", "task", "feat"), "has no Plans items", "**Books first.**", "stop and tell the user what is stale", "draft Plans")
}

func TestWorkTaskAllDone(t *testing.T) {
	root := t.TempDir()
	task(t, root, "feat", "- [x] one")
	wantAll(t, render(t, root, "work", "task", "feat"), "all 1 Plans items are marked done", "lgrass how task close feat")
}

func TestWorkTaskBuildsOnNotes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/idea/notes.md", "loose")
	wantAll(t, render(t, root, "work", "task", "idea"), "exists without a prd.md (has notes.md)", "Build on what the files")
}

func TestWorkRefusesArchivedSlug(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/archive/old/prd.md", "x")
	for _, kind := range []string{"task", "research", "debug"} {
		wantAll(t, render(t, root, "work", kind, "old"), "is an archived task")
	}
}

func TestWorkResearchAndDebugBranch(t *testing.T) {
	root := t.TempDir()
	wantAll(t, render(t, root, "work", "research", "q"), "New research q.", "Then read `biblio/books/toc.md`", "`biblio/scratchpad/q/recon.md`")
	writeFile(t, root, "biblio/scratchpad/q/recon.md", "x")
	out := render(t, root, "work", "research", "q")
	wantAll(t, out, "Continue research q.", "continue from what is still open", "lgrass how book write")
	if strings.Contains(out, "Create `") {
		t.Error("continue carries the start step")
	}
	wantAll(t, render(t, root, "work", "debug", "b"), "New debug b.", "symptom as observed", "wait for the go")
	writeFile(t, root, "biblio/scratchpad/b/debug.md", "x")
	wantAll(t, render(t, root, "work", "debug", "b"), "Continue debug b.", "current theory")
}

func TestWorkDiscussionIsAManual(t *testing.T) {
	root := t.TempDir()
	wantAll(t, render(t, root, "work", "discussion"), "1. **Books first.** Read `biblio/books/toc.md`", "needs none of this", "notes.md", "lgrass how work task <slug>")
	if _, err := Render(root, []string{"work", "discussion", "x"}); err == nil {
		t.Error("discussion accepted an argument")
	}
}
