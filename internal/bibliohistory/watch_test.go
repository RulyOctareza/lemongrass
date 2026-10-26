// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package bibliohistory

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/project"
)

const testDebounce = 150 * time.Millisecond

type watchFixture struct {
	t        *testing.T
	gitRoot  string
	mu       sync.Mutex
	projects []project.Project
}

func newWatchFixture(t *testing.T) *watchFixture {
	return &watchFixture{t: t, gitRoot: t.TempDir()}
}

func (f *watchFixture) addProject(id string) string {
	f.t.Helper()
	path := f.t.TempDir()
	if err := os.MkdirAll(filepath.Join(path, "biblio"), 0o755); err != nil {
		f.t.Fatal(err)
	}
	f.mu.Lock()
	f.projects = append(f.projects, project.Project{ID: id, Path: path})
	f.mu.Unlock()
	return filepath.Join(path, "biblio")
}

func (f *watchFixture) repo(id, biblio string) *Repo {
	return New(filepath.Join(f.gitRoot, id+".git"), biblio)
}

func (f *watchFixture) start(pass time.Duration) {
	f.t.Helper()
	w := &Watcher{
		Projects: func() ([]project.Project, error) {
			f.mu.Lock()
			defer f.mu.Unlock()
			return append([]project.Project(nil), f.projects...), nil
		},
		GitDir:   func(id string) string { return filepath.Join(f.gitRoot, id+".git") },
		Logf:     f.t.Logf,
		Debounce: testDebounce,
		Pass:     pass,
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		w.Run(ctx)
		close(done)
	}()
	f.t.Cleanup(func() {
		cancel()
		<-done
	})
}

func waitCommits(t *testing.T, r *Repo, want int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if r.Exists() && commitCount(t, r) >= want {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	got := 0
	if r.Exists() {
		got = commitCount(t, r)
	}
	t.Fatalf("commits = %d, want %d", got, want)
}

func TestWatcherCommitsAtStartAndAfterEdits(t *testing.T) {
	f := newWatchFixture(t)
	biblio := f.addProject("p1")
	writeFile(t, biblio, "books/toc.md", "one\n")
	r := f.repo("p1", biblio)

	f.start(time.Hour)
	waitCommits(t, r, 1)

	writeFile(t, biblio, "books/toc.md", "two\n")
	waitCommits(t, r, 2)
}

func TestWatcherDebouncesABurstIntoOneCommit(t *testing.T) {
	f := newWatchFixture(t)
	biblio := f.addProject("p1")
	writeFile(t, biblio, "a.md", "a\n")
	r := f.repo("p1", biblio)
	f.start(time.Hour)
	waitCommits(t, r, 1)

	for i, name := range []string{"x.md", "y.md", "scratchpad/new-task/prd.md", "a.md"} {
		writeFile(t, biblio, name, string(rune('a'+i))+"\n")
		time.Sleep(testDebounce / 4)
	}
	waitCommits(t, r, 2)
	time.Sleep(3 * testDebounce)
	if got := commitCount(t, r); got != 2 {
		t.Fatalf("commits = %d, want 2", got)
	}
}

func TestWatcherWatchesNewFolders(t *testing.T) {
	f := newWatchFixture(t)
	biblio := f.addProject("p1")
	writeFile(t, biblio, "a.md", "a\n")
	r := f.repo("p1", biblio)
	f.start(time.Hour)
	waitCommits(t, r, 1)

	writeFile(t, biblio, "scratchpad/task/prd.md", "one\n")
	waitCommits(t, r, 2)
	writeFile(t, biblio, "scratchpad/task/prd.md", "two\n")
	waitCommits(t, r, 3)
}

func TestWatcherPicksUpProjectsOnPass(t *testing.T) {
	f := newWatchFixture(t)
	first := f.addProject("p1")
	writeFile(t, first, "a.md", "a\n")
	f.start(200 * time.Millisecond)
	waitCommits(t, f.repo("p1", first), 1)

	second := f.addProject("p2")
	writeFile(t, second, "b.md", "b\n")
	r := f.repo("p2", second)
	waitCommits(t, r, 1)
	writeFile(t, second, "b.md", "c\n")
	waitCommits(t, r, 2)
}

func TestWatcherStaysOffWithoutGit(t *testing.T) {
	old := systemGit
	systemGit = fakeSystem{goos: "linux"}.lookup
	gitMu.Lock()
	gitFound = ""
	gitMu.Unlock()
	t.Cleanup(func() { systemGit = old })

	f := newWatchFixture(t)
	biblio := f.addProject("p1")
	writeFile(t, biblio, "a.md", "a\n")
	var mu sync.Mutex
	var lines []string
	w := &Watcher{
		Projects: func() ([]project.Project, error) { return f.projects, nil },
		GitDir:   func(id string) string { return filepath.Join(f.gitRoot, id+".git") },
		Logf: func(format string, args ...any) {
			mu.Lock()
			lines = append(lines, fmt.Sprintf(format, args...))
			mu.Unlock()
		},
		Debounce: testDebounce,
		Pass:     50 * time.Millisecond,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	w.Run(ctx)

	if f.repo("p1", biblio).Exists() {
		t.Fatal("history created without git")
	}
	mu.Lock()
	defer mu.Unlock()
	if len(lines) != 1 || !strings.Contains(lines[0], "git is not installed") {
		t.Fatalf("log lines = %q", lines)
	}
}
