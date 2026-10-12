package how

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	if !strings.HasPrefix(out, "Light close for fix-x: no handover, no child task, ") {
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
	if !strings.HasPrefix(out, "Full close for big: handover big-handover.md") {
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
	if out := render(t, root, "task", "close", "solo"); !strings.HasPrefix(out, "Full close for solo: handover solo.md") {
		t.Fatalf("headline = %q", strings.SplitN(out, "\n", 2)[0])
	}
}

func TestFullCloseListsChildren(t *testing.T) {
	root := t.TempDir()
	task(t, root, "parent", "- [x] one")
	writeFile(t, root, "biblio/scratchpad/kid/prd.md", "Split from biblio/scratchpad/parent/prd.md, Plans.")
	writeFile(t, root, "biblio/scratchpad/kid2/sub/notes.md", "See biblio/scratchpad/parent/ for context.")
	writeFile(t, root, "biblio/scratchpad/unrelated/activities.md", "Mentions parent by name only.")
	writeFile(t, root, "biblio/scratchpad/archive/old/prd.md", "biblio/scratchpad/parent/prd.md")
	out := render(t, root, "task", "close", "parent")
	if !strings.Contains(out, "2 child task(s)") || !strings.Contains(out, "each with `lgrass how task close <child>`: kid, kid2.") {
		t.Fatalf("children not listed: %q", out)
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
		if out := render(t, root, "task", "close", slug); !strings.HasPrefix(out, want+" for "+slug+":") {
			t.Errorf("%s headline = %q, want %s", slug, strings.SplitN(out, "\n", 2)[0], want)
		}
	}
	if out := render(t, root, "task", "close", "biblio/scratchpad/six/"); !strings.Contains(out, "PRD words, 6 Plans item(s).") {
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
