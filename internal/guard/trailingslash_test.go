// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// These cases spell paths the Unix way; the Windows spellings are in the _windows_test.go files beside them.

//go:build !windows

package guard

import (
	"os"
	"path/filepath"
	"testing"
)

// trailingSlashFixture is a work dir holding links to the home, to the home's .ssh and to /usr, with the home under the same resolved temp dir.
func trailingSlashFixture(t *testing.T) (work, home string) {
	t.Helper()
	base := resolvedTempDir(t)
	home = filepath.Join(base, "home")
	work = filepath.Join(base, "work")
	for _, dir := range []string{filepath.Join(home, ".ssh"), work} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range map[string]string{"h": home, "keys": filepath.Join(home, ".ssh"), "u": "/usr"} {
		if err := os.Symlink(target, filepath.Join(work, link)); err != nil {
			t.Fatal(err)
		}
	}
	return work, home
}

func TestResolvePathFollowsATrailingSlash(t *testing.T) {
	work, home := trailingSlashFixture(t)
	usr, err := filepath.EvalSymlinks("/usr")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct{ path, want string }{
		{"h", filepath.Join(work, "h")},
		{"h/", home},
		{"./h/", home},
		{filepath.Join(work, "h") + "/", home},
		{"u/", usr},
		{"/", "/"},
		{"missing/", filepath.Join(work, "missing")},
		{"~/", home},
		{"$HOME/", home},
	}
	for _, tc := range cases {
		if got := ResolvePath(tc.path, work, home); got != tc.want {
			t.Errorf("ResolvePath(%q) = %q, want %q", tc.path, got, tc.want)
		}
	}
}

func TestNeverExecuteFollowsATrailingSlash(t *testing.T) {
	work, home := trailingSlashFixture(t)
	for _, command := range []string{"rm -rf h/", "rm -rf u/", "chmod -R 777 u/", "rm -r keys/", "rm -rf /usr/", "rm -rf /etc/"} {
		if _, refused := NeverExecute(command, work, home); !refused {
			t.Errorf("NeverExecute(%q) was not refused", command)
		}
	}
	// Without the slash only the link itself is removed.
	for _, command := range []string{"rm -rf u", "rm keys"} {
		if reason, refused := NeverExecute(command, work, home); refused {
			t.Errorf("NeverExecute(%q) was refused: %s", command, reason)
		}
	}
}

func TestDecideFollowsATrailingSlash(t *testing.T) {
	work, home := trailingSlashFixture(t)
	verdict := Decide(Input{Tool: "Bash", Command: "ls keys/", Cwd: work, Home: home}, Policy{})
	if ruleOf(verdict) != "secret-path" {
		t.Errorf("ls keys/ through a link to ~/.ssh verdict = %+v, want secret-path", verdict)
	}
}
