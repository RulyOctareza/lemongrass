// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package bibliohistory

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func snapshotAt(t *testing.T, r *Repo, at time.Time) {
	t.Helper()
	old := clock
	clock = func() time.Time { return at }
	defer func() { clock = old }()
	if !mustSnapshot(t, r, nil) {
		t.Fatalf("no commit at %v", at)
	}
}

func fileAt(t *testing.T, r *Repo, rev, path string) string {
	t.Helper()
	out, err := r.run(nil, nil, "cat-file", "blob", rev+":"+path)
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

func TestPruneKeepsWindowAndStateAtCutoff(t *testing.T) {
	r, work := newTestRepo(t)
	now := time.Now()
	day := 24 * time.Hour

	writeFile(t, work, "books/a.md", "100 days\n")
	writeFile(t, work, "books/gone.md", "deleted later\n")
	snapshotAt(t, r, now.Add(-100*day))
	writeFile(t, work, "books/a.md", "95 days\n")
	snapshotAt(t, r, now.Add(-95*day))
	if err := os.Remove(filepath.Join(work, "books", "gone.md")); err != nil {
		t.Fatal(err)
	}
	writeFile(t, work, "books/a.md", "80 days\n")
	snapshotAt(t, r, now.Add(-80*day))
	writeFile(t, work, "books/a.md", "today\n")
	snapshotAt(t, r, now)
	writeFile(t, work, "books/a.md", "not committed yet\n")

	dropped, err := r.Prune(now.Add(-90 * day))
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 {
		t.Fatalf("dropped = %d, want 1", dropped)
	}
	if got := commitCount(t, r); got != 3 {
		t.Fatalf("commits = %d, want 3", got)
	}

	out, err := r.run(nil, nil, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		t.Fatal(err)
	}
	root := strings.TrimSpace(string(out))
	if got := fileAt(t, r, root, "books/a.md"); got != "95 days\n" {
		t.Fatalf("root a.md = %q", got)
	}
	if got := fileAt(t, r, root, "books/gone.md"); got != "deleted later\n" {
		t.Fatalf("root gone.md = %q", got)
	}
	if got := fileAt(t, r, "HEAD", "books/a.md"); got != "today\n" {
		t.Fatalf("head a.md = %q", got)
	}
	when, err := r.commitTime(root)
	if err != nil {
		t.Fatal(err)
	}
	if d := when.Sub(now.Add(-95 * day)); d < -time.Second || d > time.Second {
		t.Fatalf("root date moved by %v", d)
	}

	data, err := os.ReadFile(filepath.Join(work, "books", "a.md"))
	if err != nil || string(data) != "not committed yet\n" {
		t.Fatalf("work tree touched: %q %v", data, err)
	}
	if _, err := os.Stat(filepath.Join(work, "books", "gone.md")); !os.IsNotExist(err) {
		t.Fatal("prune restored a deleted file into the work tree")
	}
	if mustSnapshot(t, r, nil); fileAt(t, r, "HEAD", "books/a.md") != "not committed yet\n" {
		t.Fatal("snapshot after prune did not commit")
	}
}

func TestPruneLeavesYoungHistoryAlone(t *testing.T) {
	r, work := newTestRepo(t)
	now := time.Now()
	writeFile(t, work, "a.md", "one\n")
	snapshotAt(t, r, now.Add(-10*24*time.Hour))
	writeFile(t, work, "a.md", "two\n")
	snapshotAt(t, r, now)
	head, _ := r.head()

	dropped, err := r.Prune(now.Add(-90 * 24 * time.Hour))
	if err != nil {
		t.Fatal(err)
	}
	after, _ := r.head()
	if dropped != 0 || after != head {
		t.Fatalf("dropped = %d, head moved %v", dropped, after != head)
	}
}

func TestPruneOnlyRootOlderThanCutoff(t *testing.T) {
	r, work := newTestRepo(t)
	now := time.Now()
	writeFile(t, work, "a.md", "one\n")
	snapshotAt(t, r, now.Add(-200*24*time.Hour))
	writeFile(t, work, "a.md", "two\n")
	snapshotAt(t, r, now)
	head, _ := r.head()

	if dropped, err := r.Prune(now.Add(-90 * 24 * time.Hour)); err != nil || dropped != 0 {
		t.Fatalf("dropped = %d, err = %v", dropped, err)
	}
	if after, _ := r.head(); after != head {
		t.Fatal("head moved")
	}
}

func TestPruneEmptyRepository(t *testing.T) {
	r, _ := newTestRepo(t)
	if dropped, err := r.Prune(time.Now()); err != nil || dropped != 0 {
		t.Fatalf("dropped = %d, err = %v", dropped, err)
	}
}
