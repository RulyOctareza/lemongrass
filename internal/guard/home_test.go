// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"os"
	"path/filepath"
	"testing"
)

// symlinkedHomes returns a home reached through a symlinked parent, as macOS /home and Fedora Atomic /home -> /var/home are, and a home that is itself a symlink, each with the path it resolves to.
func symlinkedHomes(t *testing.T) map[string][2]string {
	t.Helper()
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	mkdir := func(parts ...string) string {
		dir := filepath.Join(append([]string{base}, parts...)...)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		return dir
	}
	link := func(target string, parts ...string) string {
		path := filepath.Join(append([]string{base}, parts...)...)
		if err := os.Symlink(target, path); err != nil {
			t.Fatal(err)
		}
		return path
	}
	parentReal := mkdir("var", "home", "ann")
	parentLink := link(filepath.Join(base, "var", "home"), "home")
	selfReal := mkdir("data", "bob")
	selfLink := link(selfReal, "bob")
	for _, home := range []string{"var/home/ann", "data/bob"} {
		mkdir(home, ".ssh")
		mkdir(home, "private")
	}
	return map[string][2]string{
		"parent is a symlink": {filepath.Join(parentLink, "ann"), parentReal},
		"home is a symlink":   {selfLink, selfReal},
	}
}

func TestNeverExecuteSymlinkedHome(t *testing.T) {
	for name, pair := range symlinkedHomes(t) {
		home, real := pair[0], pair[1]
		t.Run(name, func(t *testing.T) {
			if got := ResolveDir(home); got != real {
				t.Fatalf("ResolveDir(%q) = %q, want %q", home, got, real)
			}
			for _, command := range []string{
				"rm -rf ~",
				"rm -rf $HOME",
				"rm -rf ${HOME}/",
				"rm -rf ~/*",
				"rm -r " + home,
				"rm -r " + home + "/",
				"rm -r " + real,
				"mv ~ /elsewhere",
				"chown -R nobody ~",
				"chmod -R 777 " + real,
				"rm ~/.ssh/id_ed25519",
				"echo key >> ~/.ssh/authorized_keys",
				"echo key >> " + home + "/.ssh/authorized_keys",
			} {
				if _, refused := NeverExecute(command, filepath.Join(real, "work"), home); !refused {
					t.Errorf("NeverExecute(%q) with home %s was not refused", command, home)
				}
			}
			if reason, refused := NeverExecute("rm -rf ~/work/build", real, home); refused {
				t.Errorf("rm under the home was refused: %s", reason)
			}
		})
	}
}

func TestDecideSymlinkedHome(t *testing.T) {
	policy := Policy{Paths: []string{"~/private"}}
	for name, pair := range symlinkedHomes(t) {
		home, real := pair[0], pair[1]
		t.Run(name, func(t *testing.T) {
			cases := []struct {
				in   Input
				rule string
			}{
				{Input{Tool: "Bash", Command: "echo x >> ~/.bashrc"}, "persistence-write"},
				{Input{Tool: "Bash", Command: "echo x >> " + home + "/.bashrc"}, "persistence-write"},
				{Input{Tool: "Edit", Paths: []string{"~/.zshrc"}, Writes: true}, "persistence-write"},
				{Input{Tool: "Edit", Paths: []string{filepath.Join(real, ".zshrc")}, Writes: true}, "persistence-write"},
				{Input{Tool: "Bash", Command: "cat ~/private/a.txt"}, "custom-path"},
				{Input{Tool: "Bash", Command: "cat " + home + "/private/a.txt"}, "custom-path"},
				{Input{Tool: "Read", Paths: []string{filepath.Join(real, "private", "a.txt")}}, "custom-path"},
			}
			for _, tc := range cases {
				tc.in.Cwd, tc.in.Home = real, home
				if got := Decide(tc.in, policy); got == nil || got.RuleID != tc.rule {
					t.Errorf("%s %q %v verdict = %+v, want %s", tc.in.Tool, tc.in.Command, tc.in.Paths, got, tc.rule)
				}
			}
		})
	}
}
