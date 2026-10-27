// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package bibliohistory

import (
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/session"
)

func newTestRepo(t *testing.T) (*Repo, string) {
	t.Helper()
	dir := t.TempDir()
	work := filepath.Join(dir, "biblio")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	r := New(filepath.Join(dir, "history", "p.git"), work)
	if err := r.Init(); err != nil {
		t.Fatal(err)
	}
	return r, work
}

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustSnapshot(t *testing.T, r *Repo, sessions Sessions) bool {
	t.Helper()
	ok, err := r.Snapshot(sessions)
	if err != nil {
		t.Fatal(err)
	}
	return ok
}

func commitCount(t *testing.T, r *Repo) int {
	t.Helper()
	if _, ok := r.head(); !ok {
		return 0
	}
	out, err := r.run(nil, nil, "rev-list", "--count", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func TestSnapshotCommitsOnlyChanges(t *testing.T) {
	r, work := newTestRepo(t)
	writeFile(t, work, "books/toc.md", "one\n")

	if !mustSnapshot(t, r, nil) {
		t.Fatal("first snapshot made no commit")
	}
	if mustSnapshot(t, r, nil) {
		t.Fatal("unchanged tree made a commit")
	}
	writeFile(t, work, "books/toc.md", "two\n")
	if !mustSnapshot(t, r, nil) {
		t.Fatal("edit made no commit")
	}
	if got := commitCount(t, r); got != 2 {
		t.Fatalf("commits = %d, want 2", got)
	}
}

func TestSnapshotRecordsSessions(t *testing.T) {
	r, work := newTestRepo(t)
	writeFile(t, work, "laws/a.md", "a\n")
	called := false
	mustSnapshot(t, r, func([]string, time.Time) []string {
		called = true
		return []string{"s0"}
	})
	if called {
		t.Fatal("first commit asked for sessions")
	}

	writeFile(t, work, "laws/a.md", "b\n")
	writeFile(t, work, "laws/b.md", "b\n")
	var gotPaths []string
	var gotSince time.Time
	mustSnapshot(t, r, func(paths []string, since time.Time) []string {
		gotPaths, gotSince = paths, since
		return []string{"s1", "s2"}
	})
	if !reflect.DeepEqual(gotPaths, []string{"laws/a.md", "laws/b.md"}) {
		t.Fatalf("paths = %v", gotPaths)
	}
	if gotSince.IsZero() || time.Since(gotSince) > time.Minute {
		t.Fatalf("since = %v", gotSince)
	}
	out, err := r.run(nil, nil, "log", "-1", "--format=%(trailers:key=Session,valueonly,separator=%x2C)")
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "s1,s2" {
		t.Fatalf("trailers = %q", got)
	}
}

func TestSnapshotIgnoresInheritedGitEnv(t *testing.T) {
	r, work := newTestRepo(t)
	t.Setenv("GIT_DIR", filepath.Join(t.TempDir(), "elsewhere.git"))
	t.Setenv("GIT_INDEX_FILE", filepath.Join(t.TempDir(), "index"))
	writeFile(t, work, "a.md", "a\n")
	if !mustSnapshot(t, r, nil) {
		t.Fatal("no commit")
	}
	if commitCount(t, r) != 1 {
		t.Fatal("commit did not land in the history repository")
	}
}

func TestSnapshotMissingWorkTree(t *testing.T) {
	r, work := newTestRepo(t)
	if err := os.Remove(work); err != nil {
		t.Fatal(err)
	}
	if mustSnapshot(t, r, nil) {
		t.Fatal("missing work tree made a commit")
	}
}

func TestAttribute(t *testing.T) {
	project := t.TempDir()
	biblio := filepath.Join(project, "biblio")
	touches := []session.FileTouch{
		{SessionID: "other", FilePath: filepath.Join(project, "main.go")},
		{SessionID: "abs", FilePath: filepath.Join(biblio, "books", "toc.md")},
		{SessionID: "rel", FilePath: "biblio/laws/a.md"},
		{SessionID: "abs", FilePath: filepath.Join(biblio, "laws", "a.md")},
		{SessionID: "unrelated", FilePath: filepath.Join(biblio, "laws", "b.md")},
	}
	got := Attribute(project, touches, []string{"books/toc.md", "laws/a.md"})
	if !reflect.DeepEqual(got, []string{"abs", "rel"}) {
		t.Fatalf("got %v", got)
	}
}
