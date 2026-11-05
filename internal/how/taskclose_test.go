// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeFile(t *testing.T, root, rel, body string) {
	t.Helper()
	p := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func task(t *testing.T, root, slug string, plans ...string) {
	t.Helper()
	writeFile(t, root, "biblio/scratchpad/"+slug+"/prd.md", "# T\n\n## Plans\n\n"+strings.Join(plans, "\n")+"\n")
}

func render(t *testing.T, root string, args ...string) string {
	t.Helper()
	out, err := Render(root, args)
	if err != nil {
		t.Fatalf("Render(%v): %v", args, err)
	}
	return out
}

func TestListings(t *testing.T) {
	root := t.TempDir()
	if out := render(t, root); !strings.Contains(out, "task") {
		t.Fatalf("domains = %q", out)
	}
	if out := render(t, root, "task"); !strings.Contains(out, "close") {
		t.Fatalf("actions = %q", out)
	}
	if _, err := Render(root, []string{"nope"}); err == nil || !strings.HasPrefix(err.Error(), "how: ") {
		t.Fatalf("unknown domain err = %v", err)
	}
	if _, err := Render(root, []string{"task", "nope"}); err == nil {
		t.Fatal("unknown action accepted")
	}
}

func TestLightCloseForSmallTask(t *testing.T) {
	root := t.TempDir()
	task(t, root, "fix-x", "- [x] patch it")
	out := render(t, root, "task", "close", "fix-x")
	if !strings.HasPrefix(out, "Light close for fix-x, a build task: no handover, no related task, ") {
		t.Fatalf("headline = %q", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "mv biblio/scratchpad/fix-x biblio/scratchpad/archive/") {
		t.Fatal("archive step missing the slug")
	}
	for _, absent := range []string{"Full close", "Handover", "Children", "Out of scope", "{"} {
		if strings.Contains(out, absent) {
			t.Fatalf("light close mentions %q", absent)
		}
	}
}

func TestFullCloseFromWhiteboardHandover(t *testing.T) {
	root := t.TempDir()
	task(t, root, "big", "- [x] one")
	writeFile(t, root, "biblio/handover/whiteboard.md", "1. other [a] -> other.md\n2. big [b,c] -> big-handover.md\n")
	out := render(t, root, "task", "close", "big")
	if !strings.HasPrefix(out, "Full close for big, a build task: handover big-handover.md") {
		t.Fatalf("headline = %q", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(out, "biblio/handover/big-handover.md") || !strings.Contains(out, "drop the `big` row") {
		t.Fatal("handover step not filled in")
	}
	if strings.Contains(out, "Children") {
		t.Fatal("children step shown without children")
	}
}

func TestFullCloseFromHandoverFileAlone(t *testing.T) {
	root := t.TempDir()
	task(t, root, "solo", "- [x] one")
	writeFile(t, root, "biblio/handover/solo.md", "# solo")
	if out := render(t, root, "task", "close", "solo"); !strings.HasPrefix(out, "Full close for solo, a build task: handover solo.md") {
		t.Fatalf("headline = %q", strings.SplitN(out, "\n", 2)[0])
	}
}

// started gives each listed task a biblio history start, minutes apart in the order given.
func started(root string, slugs ...string) Env {
	base := time.Date(2026, 11, 5, 10, 0, 0, 0, time.Local)
	at := map[string]time.Time{}
	for i, s := range slugs {
		at[s] = base.Add(time.Duration(i) * time.Minute)
	}
	return Env{Root: root, TaskStarted: func(slug string) (time.Time, bool) {
		t, ok := at[slug]
		return t, ok
	}}
}

func renderEnv(t *testing.T, env Env, args ...string) string {
	t.Helper()
	out, err := RenderEnv(env, args)
	if err != nil {
		t.Fatalf("RenderEnv(%v): %v", args, err)
	}
	return out
}

// No task waits on another: the split pair from the field, where the parent's Plans point at the child and the child
// says it was split from the parent, closes in either order, and each side is named as advice with its timing.
func TestRelatedTasksAreAdvice(t *testing.T) {
	root := t.TempDir()
	task(t, root, "parent", "- [x] phase 7 split into its own task, biblio/scratchpad/kid/prd.md")
	writeFile(t, root, "biblio/scratchpad/kid/prd.md", "Split from biblio/scratchpad/parent/prd.md, Plans.\n- [x] done")
	writeFile(t, root, "biblio/scratchpad/brief/notes.md", "See the parent task for context.")
	writeFile(t, root, "biblio/scratchpad/parent-two/notes.md", "A different slug that starts the same.")
	writeFile(t, root, "biblio/books/b/c.md", "Built by biblio/scratchpad/parent/prd.md.")
	env := started(root, "brief", "parent", "kid", "parent-two")

	out := renderEnv(t, env, "task", "close", "parent")
	for _, want := range []string{
		"Light close for parent, a build task: no handover, 2 related task(s),",
		"   - `kid` (newer, could be a child; it says it was split from this task):\n     - `biblio/scratchpad/kid/prd.md:1`: Split from",
		"     - `biblio/scratchpad/parent/prd.md:5`: - [x] phase 7 split into its own task",
		"   - `brief` (older, could be a parent):\n     - `biblio/scratchpad/brief/notes.md:1`: See the parent task for context.",
		"   - `biblio/books/b/c.md:1`: Built by biblio/scratchpad/parent/prd.md.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "parent-two") || strings.Contains(out, "Children") {
		t.Fatalf("over-matched or blocked: %q", out)
	}
	out = renderEnv(t, env, "task", "close", "kid")
	if !strings.Contains(out, "   - `parent` (older, could be a parent; this task says it was split from it):") || !strings.Contains(out, "## Steps") {
		t.Fatalf("kid close: %q", out)
	}
}

func TestRelatedTasksOnTheSameDay(t *testing.T) {
	root := t.TempDir()
	task(t, root, "a", "- [x] done")
	task(t, root, "b", "- [x] see biblio/scratchpad/a/prd.md")
	for _, slug := range []string{"a", "b"} {
		writeFile(t, root, "biblio/scratchpad/"+slug+"/activities.md", "- 2026-11-05: started\n")
	}
	if out := render(t, root, "task", "close", "a"); !strings.Contains(out, "   - `b` (same age):") {
		t.Fatalf("same-day label: %q", out)
	}
}

// memory stands in for the session store's per-tab close record.
func memory(env Env, now *time.Time) Env {
	seen := map[string]time.Time{}
	env.LastClose = func(slug, sig string) (time.Time, bool) {
		at, ok := seen[slug+":"+sig]
		return at, ok
	}
	env.RecordClose = func(slug, sig string) { seen[slug+":"+sig] = *now }
	return env
}

func TestRepeatCallPrintsTitlesOnly(t *testing.T) {
	root := t.TempDir()
	task(t, root, "fix", "- [x] patch")
	writeFile(t, root, "biblio/books/b/c.md", "Mentions fix here.")
	now := time.Now()
	env := memory(Env{Root: root}, &now)

	first := renderEnv(t, env, "task", "close", "fix")
	if !strings.Contains(first, "Git is read-only") || !strings.Contains(first, "## Books") {
		t.Fatalf("first call is not full: %q", first)
	}
	second := renderEnv(t, env, "task", "close", "fix")
	for _, want := range []string{"Light close for fix", "same as in your last close", "1. **Done.**\n", "   - `biblio/books/b/c.md:1`: Mentions fix here."} {
		if !strings.Contains(second, want) {
			t.Errorf("repeat lacks %q: %q", want, second)
		}
	}
	if strings.Contains(second, "Git is read-only") || strings.Contains(second, "## Books") || strings.Contains(second, "Every Plans item") {
		t.Fatalf("repeat carries the instructions: %q", second)
	}

	now = time.Now().Add(-6 * time.Minute)
	renderEnv(t, env, "task", "close", "fix")
	if out := renderEnv(t, env, "task", "close", "fix"); !strings.Contains(out, "Git is read-only") {
		t.Fatalf("a call after the window is not full: %q", out)
	}

	now = time.Now()
	writeFile(t, root, "biblio/handover/fix.md", "# fix")
	if out := renderEnv(t, env, "task", "close", "fix"); !strings.HasPrefix(out, "Full close") || !strings.Contains(out, "Git is read-only") {
		t.Fatalf("changed steps did not print in full: %q", out)
	}
}

func TestCloseByKind(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/look/recon.md", "# Q\nfindings")
	writeFile(t, root, "biblio/scratchpad/chase/debug.md", "symptom\nRoot cause: a stale cache.")
	writeFile(t, root, "biblio/scratchpad/stuck/debug.md", "symptom only")
	writeFile(t, root, "biblio/scratchpad/idea/notes.md", "maybe")
	cases := map[string]string{"look": "A recon graduates", "chase": "names the root cause", "idea": "was never built"}
	for slug, want := range cases {
		if out := render(t, root, "task", "close", slug); !strings.Contains(out, want) {
			t.Errorf("%s close lacks %q: %q", slug, want, out)
		}
	}
	out := render(t, root, "task", "close", "stuck")
	if !strings.Contains(out, "names no root cause") || strings.Contains(out, "## Steps") {
		t.Fatalf("debug without root cause: %q", out)
	}
}

func TestFootprintFromEditsAndRelatedChapters(t *testing.T) {
	root := t.TempDir()
	task(t, root, "fix", "- [x] patch")
	writeFile(t, root, "biblio/books/store/sessions.md", "The store lives in internal/store and opens lazily.")
	writeFile(t, root, "biblio/books/store/other.md", "Nothing related; internal/storekeeper is elsewhere.")
	env := Env{Root: root, EditsSince: func(time.Time) []Edit {
		return []Edit{
			{"s1", filepath.Join(root, "biblio/scratchpad/fix/prd.md")},
			{"s1", "internal/store/open.go"},
			{"s2", filepath.Join(root, "cmd/unrelated.go")},
			{"s1", filepath.Join(root, "biblio/books/store/sessions.md")},
		}
	}}
	out := renderEnv(t, env, "task", "close", "fix")
	if !strings.Contains(out, "1 code file(s)") || !strings.Contains(out, "- `internal/store/open.go`") || strings.Contains(out, "unrelated.go") {
		t.Fatalf("footprint: %q", out)
	}
	if !strings.Contains(out, "- `biblio/books/store/sessions.md` (names internal/store)") || strings.Contains(out, "other.md") {
		t.Fatalf("related chapters: %q", out)
	}
}

func TestFootprintFallsBackToNamedFiles(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/fix/prd.md", "Change `internal/store/open.go` and internal/missing.go.\n- [x] done")
	writeFile(t, root, "internal/store/open.go", "package store")
	out := render(t, root, "task", "close", "fix")
	if !strings.Contains(out, "Files the task's documents name:") || !strings.Contains(out, "- `internal/store/open.go`") || strings.Contains(out, "missing.go") {
		t.Fatalf("named footprint: %q", out)
	}
}

func TestNewFileSendsToFullClose(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	if out, err := exec.Command("git", "-C", root, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v %s", err, out)
	}
	writeFile(t, root, "biblio/scratchpad/fix/prd.md", "Add internal/store/open.go.\n- [x] done")
	writeFile(t, root, "internal/store/open.go", "package store")
	out := render(t, root, "task", "close", "fix")
	if !strings.HasPrefix(out, "Full close for fix") || !strings.Contains(out, "1 code file(s), 1 new") || !strings.Contains(out, "- `internal/store/open.go` new") {
		t.Fatalf("new file close: %q", out)
	}
}

func TestScansFillTheSteps(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "biblio/scratchpad/big/prd.md", "## Objective\nOut of scope: the Windows side.\n## Plans\n- [x] one\n- [x] retry TBD")
	writeFile(t, root, "biblio/scratchpad/big/activities.md", "- 2026-11-05: The user corrected the retry order.\n")
	writeFile(t, root, "biblio/handover/big.md", "# big")
	writeFile(t, root, "biblio/scratchpad/other/notes.md", "Waits on big to land.\nbig-ish is a different word")
	writeFile(t, root, "biblio/scratchpad/bigger/notes.md", "no mention")
	out := render(t, root, "task", "close", "big")
	for _, want := range []string{
		"   - Out of scope: the Windows side.",
		"   - - 2026-11-05: The user corrected the retry order.",
		"   - - [x] retry TBD",
		"   - `biblio/scratchpad/other/notes.md:1`: Waits on big to land.",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in %q", want, out)
		}
	}
	if strings.Contains(out, "notes.md:2") || strings.Contains(out, "handover/big.md:") {
		t.Fatalf("sweep over-matched: %q", out)
	}
}

func TestPRDSizeChoosesThePath(t *testing.T) {
	root := t.TempDir()
	items := func(n int) []string {
		var out []string
		for i := 0; i < n; i++ {
			out = append(out, "- [x] item")
		}
		return out
	}
	task(t, root, "six", items(6)...)
	task(t, root, "seven", items(7)...)
	task(t, root, "wordy", "- [x] "+strings.Repeat("word ", 500))
	cases := map[string]string{"six": "Light close", "seven": "Full close", "wordy": "Full close"}
	for slug, want := range cases {
		if out := render(t, root, "task", "close", slug); !strings.HasPrefix(out, want+" for "+slug+",") {
			t.Errorf("%s headline = %q, want %s", slug, strings.SplitN(out, "\n", 2)[0], want)
		}
	}
	if out := render(t, root, "task", "close", "biblio/scratchpad/six/"); !strings.Contains(out, "PRD words, 6 Plans item(s), ") {
		t.Errorf("headline lacks the size facts: %q", strings.SplitN(out, "\n", 2)[0])
	}
}

func TestUncheckedItemsStopBeforeTheSteps(t *testing.T) {
	root := t.TempDir()
	task(t, root, "wip", "- [x] done", `- \[ \] escaped open`, "- [ ] plain open")
	out := render(t, root, "task", "close", "wip")
	if !strings.HasPrefix(out, "wip is not ready to close.") || !strings.Contains(out, "- escaped open") || !strings.Contains(out, "- plain open") {
		t.Fatalf("notice = %q", out)
	}
	if strings.Contains(out, "## Steps") {
		t.Fatal("steps printed for an unfinished task")
	}
}

func TestCloseArgumentErrors(t *testing.T) {
	root := t.TempDir()
	for _, args := range [][]string{
		{"task", "close"},
		{"task", "close", "missing"},
		{"task", "close", "a", "b"},
		{"task", "close", "a", "--full"},
		{"task", "close", "archive"},
	} {
		if _, err := Render(root, args); err == nil || !strings.HasPrefix(err.Error(), "how: ") {
			t.Fatalf("Render(%v) err = %v", args, err)
		}
	}
}
