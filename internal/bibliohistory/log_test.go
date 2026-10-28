// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package bibliohistory

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func move(t *testing.T, root, from, to string) {
	t.Helper()
	dst := filepath.Join(root, filepath.FromSlash(to))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(root, filepath.FromSlash(from)), dst); err != nil {
		t.Fatal(err)
	}
}

func paths(e Entry) string {
	var ps []string
	for _, c := range e.Changes {
		if c.OldPath != "" {
			ps = append(ps, c.Status+" "+c.OldPath+">"+c.Path)
		} else {
			ps = append(ps, c.Status+" "+c.Path)
		}
	}
	return strings.Join(ps, ", ")
}

// A task folder edited twice, archived, then edited once more in the archive.
func archivedTask(t *testing.T) (*Repo, []string) {
	t.Helper()
	r, work := newTestRepo(t)
	writeFile(t, work, "books/toc.md", "toc\n")
	writeFile(t, work, "scratchpad/task/prd.md", "plan v1\nline two\nline three\n")
	writeFile(t, work, "scratchpad/task/activities.md", "log\n")
	mustSnapshot(t, r, nil)
	writeFile(t, work, "scratchpad/task/prd.md", "plan v2\nline two\nline three\n")
	mustSnapshot(t, r, nil)
	writeFile(t, work, "books/toc.md", "toc 2\n")
	mustSnapshot(t, r, nil)
	move(t, work, "scratchpad/task", "scratchpad/archive/task")
	mustSnapshot(t, r, nil)
	writeFile(t, work, "scratchpad/archive/task/activities.md", "log\nclosed\n")
	mustSnapshot(t, r, nil)

	var hashes []string
	entries, err := r.Log("", 10)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		hashes = append(hashes, e.Hash)
	}
	return r, hashes
}

func TestLogFollowsAFileIntoTheArchive(t *testing.T) {
	r, _ := archivedTask(t)
	entries, err := r.Log("scratchpad/archive/task/prd.md", 20)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, e := range entries {
		got = append(got, paths(e))
	}
	want := []string{
		"R scratchpad/task/prd.md>scratchpad/archive/task/prd.md",
		"M scratchpad/task/prd.md",
		"A scratchpad/task/prd.md",
	}
	if strings.Join(got, "|") != strings.Join(want, "|") {
		t.Fatalf("got %q", got)
	}
	old, err := r.Show(entries[2].Hash, "scratchpad/task/prd.md")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(old), "plan v1") {
		t.Fatalf("show = %q", old)
	}
}

func TestLogFollowsAFolderIntoTheArchive(t *testing.T) {
	r, _ := archivedTask(t)
	entries, err := r.Log("scratchpad/archive/task", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 4 {
		var got []string
		for _, e := range entries {
			got = append(got, paths(e))
		}
		t.Fatalf("entries = %d: %q", len(entries), got)
	}
	if !strings.Contains(paths(entries[1]), "R scratchpad/task/activities.md>scratchpad/archive/task/activities.md") {
		t.Fatalf("move entry = %q", paths(entries[1]))
	}
	if got := paths(entries[3]); !strings.Contains(got, "A scratchpad/task/prd.md") || strings.Contains(got, "books/toc.md") {
		t.Fatalf("first entry = %q", got)
	}
}

func TestLogLimitAndWholeHistory(t *testing.T) {
	r, hashes := archivedTask(t)
	if len(hashes) != 5 {
		t.Fatalf("whole history = %d commits", len(hashes))
	}
	entries, err := r.Log("scratchpad/archive/task/prd.md", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Hash != hashes[1] {
		t.Fatalf("limited = %+v", entries)
	}
	all, _ := r.Log("", 2)
	if len(all) != 2 || all[0].Hash != hashes[0] {
		t.Fatalf("limited whole = %+v", all)
	}
}

func TestLogCarriesSessionsAndTime(t *testing.T) {
	r, work := newTestRepo(t)
	writeFile(t, work, "a.md", "a\n")
	mustSnapshot(t, r, nil)
	writeFile(t, work, "a.md", "b\n")
	mustSnapshot(t, r, func([]string, time.Time) []string { return []string{"s1", "s2"} })
	entries, err := r.Log("a.md", 5)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || strings.Join(entries[0].Sessions, ",") != "s1,s2" || entries[1].Sessions != nil {
		t.Fatalf("entries = %+v", entries)
	}
	if time.Since(entries[0].Time) > time.Minute {
		t.Fatalf("time = %v", entries[0].Time)
	}
}

func TestShowRejects(t *testing.T) {
	r, hashes := archivedTask(t)
	cases := [][3]string{
		{"HEAD", "books/toc.md", "not a commit id"},
		{"--output=x", "books/toc.md", "not a commit id"},
		{hashes[0], "books/missing.md", "does not exist"},
		{hashes[0], "books", "is a folder"},
	}
	for _, c := range cases {
		want := c[2]
		_, err := r.Show(c[0], c[1])
		if err == nil || !strings.Contains(err.Error(), want) || !strings.HasPrefix(err.Error(), "bibliohistory: ") {
			t.Errorf("%s: err = %v", want, err)
		}
	}
}

func TestNoHistory(t *testing.T) {
	r := New(filepath.Join(t.TempDir(), "none.git"), t.TempDir())
	if _, err := r.Log("", 5); !errors.Is(err, ErrNoHistory) {
		t.Fatalf("log err = %v", err)
	}
	if _, err := r.Show("abcd", "a.md"); !errors.Is(err, ErrNoHistory) {
		t.Fatalf("show err = %v", err)
	}
}

func TestRelPath(t *testing.T) {
	project := t.TempDir()
	ok := map[string]string{
		"":                                       "",
		"books/toc.md":                           "books/toc.md",
		"biblio/books/toc.md":                    "books/toc.md",
		"./biblio/books/":                        "books",
		"biblio":                                 "",
		filepath.Join(project, "biblio", "laws"): "laws",
	}
	for in, want := range ok {
		got, err := RelPath(project, in)
		if err != nil || got != want {
			t.Errorf("RelPath(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"../secret", "biblio/../../x", filepath.Join(project, "main.go"), "/etc/passwd"} {
		if got, err := RelPath(project, in); err == nil {
			t.Errorf("RelPath(%q) = %q, want an error", in, got)
		}
	}
}
