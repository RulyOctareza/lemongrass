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
	for _, want := range []string{"lgrass workgroup add <path-to-config>", "thinkers:", "dump/", "must not carry `name`"} {
		if !strings.Contains(out, want) {
			t.Errorf("addmember missing %q", want)
		}
	}
	if _, err := Render(root, []string{"leader", "addmember", "extra"}); err == nil || !strings.HasPrefix(err.Error(), "how: ") {
		t.Errorf("extra argument err = %v", err)
	}
}
