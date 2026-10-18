package hook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTempWritesAreDeniedInABibliothekProject(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	dir := projectWithBiblio(t)
	target := filepath.Join(os.TempDir(), "lgrass-tmpgate-test", "plan.md")

	for _, call := range []struct {
		tool  string
		input map[string]string
	}{
		{"Write", map[string]string{"file_path": target, "content": "wip"}},
		{"Edit", map[string]string{"file_path": target}},
	} {
		event := preTool(gateTabOutside, call.tool, call.input)
		event.Cwd = dir
		result := hookPreToolUse(store, event, dir)
		if result.PermissionDecision != "deny" || !strings.Contains(result.PermissionDecisionReason, "biblio/scratchpad/<task-slug>/dump/") {
			t.Errorf("%s into the temp dir was not denied with the dump pointer: %+v", call.tool, result)
		}
	}
}

func TestTempGateLeavesReadsShellWritesPlainProjectsAndScratchpadAlone(t *testing.T) {
	store, _ := storeWithGroup(t, nil)
	biblioDir := projectWithBiblio(t)
	plainDir := t.TempDir()
	target := filepath.Join(os.TempDir(), "lgrass-tmpgate-test", "plan.md")

	cases := []struct {
		name    string
		project string
		tool    string
		input   map[string]string
	}{
		{"read in a bibliothek project", biblioDir, "Read", map[string]string{"file_path": target}},
		{"cat in a bibliothek project", biblioDir, "Bash", map[string]string{"command": "cat " + target}},
		{"shell write in a bibliothek project", biblioDir, "Bash", map[string]string{"command": "go test ./... > " + target}},
		{"write in a plain project", plainDir, "Write", map[string]string{"file_path": target, "content": "wip"}},
		{"write into the task dump", biblioDir, "Write", map[string]string{"file_path": filepath.Join(biblioDir, "biblio", "scratchpad", "task", "dump", "plan.md"), "content": "wip"}},
	}
	for _, c := range cases {
		event := preTool(gateTabOutside, c.tool, c.input)
		event.Cwd = c.project
		if result := hookPreToolUse(store, event, c.project); result.PermissionDecision == "deny" {
			t.Errorf("%s was denied: %s", c.name, result.PermissionDecisionReason)
		}
	}
}
