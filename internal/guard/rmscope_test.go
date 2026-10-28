// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func rmProject(t *testing.T) (root, home, outside string) {
	t.Helper()
	home = t.TempDir()
	root = filepath.Join(home, "work", "app")
	outside = t.TempDir()
	for _, dir := range []string{"build/out", "sub", ".git/refs", "biblio/books"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"a.log", "b.log", "notes.txt", "sub/c.txt", ".git/index.lock", "biblio/books/toc.md"} {
		if err := os.WriteFile(filepath.Join(root, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	return root, home, outside
}

func TestRmInsideProject(t *testing.T) {
	root, home, outside := rmProject(t)
	cases := []struct {
		command string
		cwd     string
		want    string
	}{
		{"rm notes.txt", "", ""},
		{"rm -rf build", "", ""},
		{"rm -f a.log b.log", "", ""},
		{"rm *.log", "", ""},
		{"rm -r -- build/out", "", ""},
		{"rm -rf " + filepath.Join(root, "build"), "", ""},
		{"rm -rf ~/work/app/build", "", ""},
		{"rm -rf $HOME/work/app/build", "", ""},
		{"rm -rf \"${HOME}/work/app/build\"", "", ""},
		{"rm -rf build && go build ./...", "", ""},
		{"bash -c 'rm -rf build'", "", ""},
		{"rm c.txt", "sub", ""},
		{"rm -rf .", "sub", ""},

		{"rm -rf " + outside, "", "rm-force"},
		{"rm " + filepath.Join(outside, "x"), "", "rm-plain"},
		{"rm -rf ~/other", "", "rm-force"},
		{"rm -rf ~", "", "rm-force"},
		{"rm -rf ~root/x", "", "rm-force"},
		{"rm -rf ../app", "", "rm-force"},
		{"rm -rf build/../..", "", "rm-force"},
		{"rm -rf escape", "", "rm-force"},
		{"rm -rf escape/x", "", "rm-force"},
		{"rm -rf .", "", "rm-force"},
		{"rm -rf " + root, "", "rm-force"},
		{"rm -rf /*", "", "rm-force"},
		{"rm -rf .git", "", "rm-force"},
		{"rm .git/index.lock", "", "rm-plain"},
		{"rm -rf biblio", "", "rm-force"},
		{"rm biblio/books/toc.md", "", "rm-plain"},
		{"rm -rf *", "", "rm-force"},
		{"rm -rf .*", "", "rm-force"},
		{"rm -rf \"$TMP\"/build", "", "rm-force"},
		{"rm -rf $(pwd)/build", "", "rm-force"},
		{"rm -rf {build,/etc}", "", "rm-force"},
		{"rm -rf --no-preserve-root build", "", "rm-force"},
		{"rm -rf", "", "rm-force"},
		{"cd build && rm -rf out", "", ""},
		{"(cd /tmp; rm -rf build)", "", "rm-force"},
		{"pushd sub && rm c.txt", "", ""},
		{"env -C /tmp rm -rf build", "", "rm-force"},
		{"ls | xargs rm -f", "", "rm-force"},
		{"echo build | xargs rm -rf build", "", "rm-force"},
		{"find . -name '*.tmp' -exec rm {} \\;", "", "rm-plain"},
		{"rm -rf build", outside, "rm-force"},

		{"find build -name '*.o' -delete", "", ""},
		{"find build sub -type f -delete", "", ""},
		{"find -P build -delete", "", ""},
		{"find " + filepath.Join(root, "build") + " -delete", "", ""},
		{"find ~/work/app/build -delete", "", ""},
		{"find . -name '*.tmp' -delete", "sub", ""},
		{"find . -name '*.pyc' -delete", "", "find-delete"},
		{"find -name '*.pyc' -delete", "", "find-delete"},
		{"find " + root + " -delete", "", "find-delete"},
		{"find .git -name '*.lock' -delete", "", "find-delete"},
		{"find biblio -delete", "", "find-delete"},
		{"find build biblio -delete", "", "find-delete"},
		{"find " + outside + " -delete", "", "find-delete"},
		{"find escape -delete", "", "find-delete"},
		{"find -L build -delete", "", "find-delete"},
		{"find build -follow -delete", "", "find-delete"},
		{"find \"$DIR\" -delete", "", "find-delete"},
		{"cd build && find out -delete", "", ""},
		{"find build -delete", outside, "find-delete"},
		{"find build -name x -exec rm {} \\;", "", "rm-plain"},
	}
	for _, tc := range cases {
		t.Run(tc.command+" in "+tc.cwd, func(t *testing.T) {
			cwd := root
			if filepath.IsAbs(tc.cwd) {
				cwd = tc.cwd
			} else if tc.cwd != "" {
				cwd = filepath.Join(root, tc.cwd)
			}
			in := Input{Tool: "Bash", Command: tc.command, Cwd: cwd, Home: home, ProjectRoot: root}
			got := ""
			if verdict := Decide(in, Policy{}); verdict != nil {
				got = verdict.RuleID
			}
			if got != tc.want {
				t.Errorf("rule = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestRmWithoutProjectRootKeepsGate(t *testing.T) {
	root, home, _ := rmProject(t)
	in := Input{Tool: "Bash", Command: "rm -rf build", Cwd: root, Home: home}
	if verdict := Decide(in, Policy{}); verdict == nil || verdict.RuleID != "rm-force" {
		t.Errorf("verdict = %+v, want rm-force", verdict)
	}
}

func TestRmInsideProjectHonorsUserDeny(t *testing.T) {
	root, home, _ := rmProject(t)
	in := Input{Tool: "Bash", Command: "rm notes.txt", Cwd: root, Home: home, ProjectRoot: root}
	policy := Policy{Rules: map[string]string{"rm-plain": overrideDeny}}
	if verdict := Decide(in, policy); verdict == nil || verdict.RuleID != "rm-plain" || verdict.Mode != ModeDeny {
		t.Errorf("verdict = %+v, want an rm-plain deny", verdict)
	}
}

func TestRmInsideProjectKeepsSecretCheck(t *testing.T) {
	root, home, _ := rmProject(t)
	if err := os.MkdirAll(filepath.Join(root, ".ssh"), 0o755); err != nil {
		t.Fatal(err)
	}
	in := Input{Tool: "Bash", Command: "rm -rf .ssh", Cwd: root, Home: home, ProjectRoot: root}
	if verdict := Decide(in, Policy{}); verdict == nil || verdict.RuleID != "secret-path" {
		t.Errorf("verdict = %+v, want secret-path", verdict)
	}
}

func TestDeletionThroughSymlinkedProjectPath(t *testing.T) {
	base := t.TempDir()
	real := filepath.Join(base, "data", "Projects", "app")
	if err := os.MkdirAll(filepath.Join(real, "build"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(real, "biblio"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(real, "a.txt"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(base, "home")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(base, "data", "Projects"), filepath.Join(home, "Projects")); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(home, "Projects", "app")
	spellings := map[string]string{"real": real, "link": link}
	for rootName, root := range spellings {
		for cwdName, cwd := range spellings {
			cases := []struct {
				command string
				want    string
			}{
				{"rm -rf a.txt", ""},
				{"rm -rf build", ""},
				{"rm -rf missing/x", ""},
				{"rm -rf " + filepath.Join(real, "missing", "x"), ""},
				{"rm -rf " + filepath.Join(link, "missing", "x"), ""},
				{"rm -rf ~/Projects/app/missing/x", ""},
				{"find missing/dir -delete", ""},
				{"rm -rf " + filepath.Join(link, "biblio", "missing"), "rm-force"},
				{"rm -rf ~/Projects/other/missing", "rm-force"},
				{"rm -rf " + link, "rm-force"},
			}
			for _, tc := range cases {
				t.Run(rootName+" root, "+cwdName+" cwd: "+tc.command, func(t *testing.T) {
					in := Input{Tool: "Bash", Command: tc.command, Cwd: cwd, Home: home, ProjectRoot: root}
					got := ""
					if verdict := Decide(in, Policy{}); verdict != nil {
						got = verdict.RuleID
					}
					if got != tc.want {
						t.Errorf("rule = %q, want %q", got, tc.want)
					}
				})
			}
		}
	}
}

func TestDeletionInsideScratchpad(t *testing.T) {
	root, home, _ := rmProject(t)
	for _, dir := range []string{"biblio/scratchpad/task-a", "biblio/scratchpad/archive/old-task"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for _, file := range []string{"biblio/scratchpad/task-a/notes.md", "biblio/scratchpad/task-a/tmp.log", "biblio/scratchpad/archive/old-task/prd.md"} {
		if err := os.WriteFile(filepath.Join(root, file), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		command string
		want    string
	}{
		{"rm biblio/scratchpad/task-a/notes.md", ""},
		{"rm -rf biblio/scratchpad/task-a", ""},
		{"rm biblio/scratchpad/task-a/*.log", ""},
		{"rm -rf biblio/scratchpad/new-idea", ""},
		{"find biblio/scratchpad/task-a -name '*.log' -delete", ""},
		{"rm -rf " + filepath.Join(root, "biblio", "scratchpad", "task-a"), ""},

		{"rm -rf biblio/scratchpad", "rm-force"},
		{"rm -rf biblio/scratchpad/", "rm-force"},
		{"rm -rf biblio/scratchpad/archive", "rm-force"},
		{"rm biblio/scratchpad/archive/old-task/prd.md", "rm-plain"},
		{"rm -rf biblio/scratchpad/*", "rm-force"},
		{"rm -rf biblio/scratchpad/task-a/../archive", "rm-force"},
		{"find biblio/scratchpad -delete", "find-delete"},
		{"rm -rf biblio/books", "rm-force"},
		{"rm biblio/books/toc.md", "rm-plain"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			in := Input{Tool: "Bash", Command: tc.command, Cwd: root, Home: home, ProjectRoot: root}
			got := ""
			if verdict := Decide(in, Policy{}); verdict != nil {
				got = verdict.RuleID
			}
			if got != tc.want {
				t.Errorf("rule = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeletionFollowsCdInsideProject(t *testing.T) {
	root, home, outside := rmProject(t)
	cases := []struct {
		command string
		want    string
	}{
		{"cd sub && rm c.txt", ""},
		{"cd biblio/scratchpad && rm -rf task-a", ""},
		{"cd build && cd out && rm x", ""},
		{"cd sub; rm c.txt", ""},
		{"(cd sub; rm c.txt)", ""},
		{"cd -P sub && rm c.txt", ""},
		{"cd sub && cd .. && rm notes.txt", "rm-plain"},

		{"cd .. && rm -rf app/build", "rm-force"},
		{"cd escape && rm x", "rm-plain"},
		{"cd " + outside + " && rm x", "rm-plain"},
		{"cd && rm x", "rm-plain"},
		{"cd - && rm x", "rm-plain"},
		{"cd ~ && rm x", "rm-plain"},
		{"cd \"$D\" && rm x", "rm-plain"},
		{"popd && rm x", "rm-plain"},
		{"cd sub && rm -rf ../.git", "rm-force"},
		{"cd sub && rm ../../x", "rm-plain"},
		{"cd biblio && rm -rf books", "rm-force"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			in := Input{Tool: "Bash", Command: tc.command, Cwd: root, Home: home, ProjectRoot: root}
			got := ""
			if verdict := Decide(in, Policy{}); verdict != nil {
				got = verdict.RuleID
			}
			if got != tc.want {
				t.Errorf("rule = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestDeletionDenialDetail(t *testing.T) {
	root, home, outside := rmProject(t)
	cases := []struct {
		command string
		cwd     string
		root    string
		policy  Policy
		want    string
	}{
		{"rm -rf ../x", root, root, Policy{}, "is outside the project " + root},
		{"rm biblio/books/toc.md", root, root, Policy{}, "is in biblio, which is protected"},
		{"rm -rf .git", root, root, Policy{}, "is in .git, which is protected"},
		{"rm -rf biblio/scratchpad", root, root, Policy{}, "is biblio/scratchpad itself"},
		{"rm -rf biblio/scratchpad/archive", root, root, Policy{}, "frozen record of closed tasks"},
		{"rm -rf .", root, root, Policy{}, "is the project root, which cannot be removed"},
		{"rm -rf \"$TMP\"/x", root, root, Policy{}, "uses a variable, substitution, braces or ~user"},
		{"cd .. && rm -rf x", root, root, Policy{}, "'cd ..' leaves the project"},
		{"cd escape && rm x", root, root, Policy{}, "'cd escape' is a symlink to " + outside},
		{"popd && rm x", root, root, Policy{}, "'popd' moves to a directory the guard cannot know"},
		{"cd \"$D\" && rm x", root, root, Policy{}, "uses a variable or substitution"},
		{"rm x", outside, root, Policy{}, "The shell is in " + outside},
		{"ls | xargs rm -f", root, root, Policy{}, "come from xargs, find -exec or env -C"},
		{"find -L build -delete", root, root, Policy{}, "find follows symlinks"},
		{"find . -name x -delete", root, root, Policy{}, "find from the project root would also search .git and biblio"},
		{"rm -rf", root, root, Policy{}, "rm has no path to check"},
		{"rm -rf --no-preserve-root build", root, root, Policy{}, "--no-preserve-root is never allowed"},
		{"rm notes.txt", root, "", Policy{}, "no launch project"},
		{"rm notes.txt", root, root, Policy{Rules: map[string]string{"rm-plain": overrideDeny}}, "Safety policy sets rm-plain to deny"},
	}
	for _, tc := range cases {
		t.Run(tc.command, func(t *testing.T) {
			in := Input{Tool: "Bash", Command: tc.command, Cwd: tc.cwd, Home: home, ProjectRoot: tc.root}
			verdict := Decide(in, tc.policy)
			if verdict == nil {
				t.Fatal("not denied")
			}
			message := verdict.Message()
			if !strings.Contains(message, tc.want) || !strings.Contains(message, "! prefix") {
				t.Errorf("message = %q, want it to contain %q", message, tc.want)
			}
		})
	}
}
