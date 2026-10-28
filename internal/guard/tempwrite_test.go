// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

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
	for _, dir := range []string{filepath.Join(project, "biblio", "scratchpad", "task"), tmp, home} {
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

func TestTempWriteLeavesShellCommandsAlone(t *testing.T) {
	project, tmp, home := tempWriteSetup(t)
	x := filepath.Join(tmp, "x")
	for _, command := range []string{
		"echo hi > " + x,
		"cat > " + x + " <<'EOF'\nwork\nEOF",
		"cargo build --target-dir " + tmp,
		"curl -o " + x + " https://example.com",
		"mktemp -d",
	} {
		if got := TempWrite(Input{Tool: "Bash", Command: command, Cwd: project, Home: home, ProjectRoot: project}, []string{tmp, "/tmp"}); got != "" {
			t.Errorf("TempWrite(%q) = %q, want shell writes left alone", command, got)
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
	inside := filepath.Join(project, "biblio", "scratchpad", "task", "plan.md")
	if got := TempWrite(Input{Tool: "Write", Paths: []string{inside}, Writes: true, Cwd: project, Home: home, ProjectRoot: project}, roots); got != "" {
		t.Errorf("a Write into the task directory was caught: %q", got)
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
