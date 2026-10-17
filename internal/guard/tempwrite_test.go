package guard

import (
	"os"
	"path/filepath"
	"testing"
)

func tempWriteSetup(t *testing.T) (project, tmp, home string) {
	t.Helper()
	base := t.TempDir()
	project = filepath.Join(base, "proj")
	tmp = filepath.Join(base, "tmp")
	home = filepath.Join(base, "home")
	for _, dir := range []string{filepath.Join(project, "biblio", "scratchpad", "task", "dump"), tmp, home} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(tmp, filepath.Join(project, "link")); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TMPDIR", tmp)
	return project, tmp, home
}

func TestTempWriteShellCommands(t *testing.T) {
	project, tmp, home := tempWriteSetup(t)
	x := filepath.Join(tmp, "x")
	cases := []struct {
		command string
		want    string
	}{
		{"echo hi > " + x, x},
		{"echo hi >> " + x, x},
		{"cat > " + x + " <<'EOF'\nwork\nEOF", x},
		{"echo hi | tee -a " + x, x},
		{"cp a.txt " + tmp, tmp},
		{"mv -f a.txt b.txt " + x, x},
		{"cp -t " + tmp + " a.txt", tmp},
		{"install -m 644 a.txt " + x, x},
		{"mkdir -p " + x, x},
		{"touch " + x, x},
		{"mktemp", "$TMPDIR/"},
		{"mktemp -d", "$TMPDIR/"},
		{"mktemp /tmp/work.XXXX", "/tmp/work.XXXX"},
		{"mktemp -t work.XXXX", "$TMPDIR/work.XXXX"},
		{"cd " + tmp + " && echo hi > notes.md", "notes.md"},
		{"echo hi > $TMPDIR/x", "$TMPDIR/x"},
		{`echo hi > "${TMPDIR}/x"`, "$TMPDIR/x"},
		{"echo hi > link/x", "link/x"},
		{"bash -c 'echo hi > " + x + "'", x},

		{"cat " + x, ""},
		{"ls " + tmp, ""},
		{"echo hi > /dev/null", ""},
		{"echo hi > biblio/scratchpad/task/dump/x", ""},
		{"cp " + x + " biblio/scratchpad/task/dump/", ""},
		{"mktemp -p biblio/scratchpad/task/dump", ""},
		{"mkdir -m 700 biblio/scratchpad/task/dump/run", ""},
		{"echo hi > $OUT/x", ""},
		{"cd " + tmp + " && ls", ""},
	}
	for _, c := range cases {
		got := TempWrite(Input{Tool: "Bash", Command: c.command, Cwd: project, Home: home, ProjectRoot: project}, []string{tmp, "/tmp"})
		if got != c.want {
			t.Errorf("TempWrite(%q) = %q, want %q", c.command, got, c.want)
		}
	}
}

func TestTempWriteFileTools(t *testing.T) {
	project, tmp, home := tempWriteSetup(t)
	roots := []string{tmp}
	x := filepath.Join(tmp, "plan.md")
	if got := TempWrite(Input{Tool: "Write", Paths: []string{x}, Writes: true, Cwd: project, Home: home, ProjectRoot: project}, roots); got != x {
		t.Errorf("a Write into the temp dir = %q, want it caught", got)
	}
	if got := TempWrite(Input{Tool: "Read", Paths: []string{x}, Cwd: project, Home: home, ProjectRoot: project}, roots); got != "" {
		t.Errorf("a Read of the temp dir was caught: %q", got)
	}
	inside := filepath.Join(project, "biblio", "scratchpad", "task", "dump", "plan.md")
	if got := TempWrite(Input{Tool: "Write", Paths: []string{inside}, Writes: true, Cwd: project, Home: home, ProjectRoot: project}, roots); got != "" {
		t.Errorf("a Write into the task dump was caught: %q", got)
	}
}

func TestTempWriteSparesAProjectInsideTheTempDir(t *testing.T) {
	_, tmp, home := tempWriteSetup(t)
	project := filepath.Join(tmp, "proj")
	os.MkdirAll(project, 0o755)
	in := Input{Tool: "Write", Paths: []string{filepath.Join(project, "main.go")}, Writes: true, Cwd: project, Home: home, ProjectRoot: project}
	if got := TempWrite(in, []string{tmp}); got != "" {
		t.Errorf("a write inside a project that lives in the temp dir was caught: %q", got)
	}
	in.Paths = []string{filepath.Join(tmp, "other.md")}
	if got := TempWrite(in, []string{tmp}); got == "" {
		t.Error("a write beside that project in the temp dir was not caught")
	}
}

func TestTempRootsIncludesClaudeTempDir(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(claudeTempDirEnv, dir)
	if !containsArg(TempRoots(), resolveFully(dir)) {
		t.Errorf("TempRoots() = %v, want %s among them", TempRoots(), dir)
	}
}
