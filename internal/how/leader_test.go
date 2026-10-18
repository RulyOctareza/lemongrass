package how

import (
	"strings"
	"testing"
)

func TestLeaderAddMemberIsListedAndRenders(t *testing.T) {
	root := t.TempDir()
	if out := render(t, root); !strings.Contains(out, "leader") {
		t.Fatalf("domains = %q", out)
	}
	if out := render(t, root, "leader"); !strings.Contains(out, "addmember") {
		t.Fatalf("actions = %q", out)
	}
	out := render(t, root, "leader", "addmember")
	for _, want := range []string{"lgrass workgroup add <path-to-config>", "thinkers:", "biblio/scratchpad/<task-slug>/add-docs.yaml", "must not carry `name`"} {
		if !strings.Contains(out, want) {
			t.Errorf("addmember missing %q", want)
		}
	}
	if _, err := Render(root, []string{"leader", "addmember", "extra"}); err == nil || !strings.HasPrefix(err.Error(), "how: ") {
		t.Errorf("extra argument err = %v", err)
	}
}

func TestCollaboratorJoinRenders(t *testing.T) {
	out := render(t, t.TempDir(), "collaborator", "join")
	for _, want := range []string{"lgrass workgroup thread --all", "Introduce yourself", "agree with that member in the thread", "tell your human"} {
		if !strings.Contains(out, want) {
			t.Errorf("collaborator join missing %q", want)
		}
	}
}
