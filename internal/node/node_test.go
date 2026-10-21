package node

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/faizalv/lemongrass/internal/project"
)

const testBook = "garden_design"

type fixture struct {
	lib   *Library
	cfg   Config
	grant Grant
	root  string
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newFixture(t *testing.T) fixture {
	t.Helper()
	dir := t.TempDir()
	root := filepath.Join(dir, "biblio")
	write(t, filepath.Join(root, "books", testBook, "beds.md"), "raised beds\n")
	write(t, filepath.Join(root, "books", testBook, "images", "a.md"), "nested")
	write(t, filepath.Join(root, "books", "secret_plans", "plan.md"), "hidden")
	write(t, filepath.Join(root, "scratchpad", "irrigation", "prd.md"), "drip lines\n")
	write(t, filepath.Join(root, "scratchpad", "irrigation", "notes.txt"), "not markdown")
	write(t, filepath.Join(root, "scratchpad", "irrigation", "app", "x.md"), "nested")
	write(t, filepath.Join(root, "scratchpad", "archive", "compost", "prd.md"), "done\n")
	write(t, filepath.Join(root, "scratchpad", "private", "prd.md"), "hidden")
	write(t, filepath.Join(root, "laws", "summary.md"), "hidden")

	lib := &Library{Projects: func() ([]project.Project, error) {
		return []project.Project{{ID: "p1", Name: "garden", Path: dir}}, nil
	}}
	grant := Grant{
		ID:        "g1",
		ExpiresAt: time.Now().Add(time.Hour),
		Shares:    []Share{{Alias: "garden", Books: []string{"garden_design"}, Tasks: []string{"irrigation", "compost", "missing"}}},
	}
	cfg := Config{Projects: []ProjectAlias{{Alias: "garden", ProjectID: "p1"}}, Grants: []Grant{grant}}
	return fixture{lib: lib, cfg: cfg, grant: grant, root: root}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	if err := png.Encode(&b, image.NewGray(image.Rect(0, 0, 2, 2))); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func (f fixture) read(t *testing.T, path string) (string, error) {
	t.Helper()
	c, err := f.lib.Read(f.cfg, f.grant, path, 0)
	return c.Text, err
}

func TestContentsListsOnlyGrantedBooksAndTasks(t *testing.T) {
	f := newFixture(t)
	got, err := f.read(t, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "garden/books/" + testBook + "\ngarden/tasks/irrigation\ngarden/tasks/compost (archived)\n"
	if got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}
}

func TestReadsBookChaptersAndTaskFiles(t *testing.T) {
	f := newFixture(t)
	cases := map[string]string{
		"garden/books/" + testBook:              "beds.md\n",
		"garden/books/" + testBook + "/beds.md": "raised beds\n",
		"garden/books/" + testBook + "/beds":    "raised beds\n",
		"garden/tasks/irrigation":               "notes.txt\nprd.md\napp/x.md\n",
		"garden/tasks/irrigation/prd.md":        "drip lines\n",
		"garden/tasks/irrigation/notes.txt":     "not markdown",
		"garden/tasks/irrigation/app":           "x.md\n",
		"garden/tasks/irrigation/app/x":         "nested",
		"garden/tasks/compost":                  "(archived task)\nprd.md\n",
		"garden/tasks/compost/prd":              "done\n",
	}
	for path, want := range cases {
		got, err := f.read(t, path)
		if err != nil {
			t.Errorf("read %q: %v", path, err)
			continue
		}
		if got != want {
			t.Errorf("read %q = %q, want %q", path, got, want)
		}
	}
}

func TestBracketFormGrantMatchesPlainFolder(t *testing.T) {
	f := newFixture(t)
	f.grant.Shares[0].Books = []string{"garden_design[2026-01-02]"}
	got, err := f.read(t, "garden/books/garden_design/beds.md")
	if err != nil || got != "raised beds\n" {
		t.Errorf("read under a bracket-form grant = %q, %v", got, err)
	}
	if got, _ := f.read(t, ""); !strings.Contains(got, "garden/books/garden_design\n") {
		t.Errorf("contents under a bracket-form grant = %q", got)
	}
}

func TestBracketFormFolderIsServed(t *testing.T) {
	f := newFixture(t)
	books := filepath.Join(f.root, "books")
	if err := os.Rename(filepath.Join(books, testBook), filepath.Join(books, "garden_design[2026-01-02][soil,water]")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"garden/books/garden_design/beds.md", "garden/books/garden_design[2026-01-02][soil,water]/beds.md"} {
		if got, err := f.read(t, p); err != nil || got != "raised beds\n" {
			t.Errorf("read %q = %q, %v", p, got, err)
		}
	}
}

func TestOutsideTheGrantIsNotFound(t *testing.T) {
	f := newFixture(t)
	paths := []string{
		"other",
		"garden/laws",
		"garden/laws/summary.md",
		"garden/books/secret_plans",
		"garden/books/secret_plans/plan.md",
		"garden/tasks/private",
		"garden/tasks/private/prd.md",
		"garden/tasks/missing",
		"garden/tasks/archive",
		"garden/tasks/irrigation/notes",
		"garden/tasks/irrigation/app/../prd.md",
		"garden/tasks/irrigation//prd.md",
		"garden/books/" + testBook + "/images",
		"garden/books/" + testBook + "/images/a.md",
		"garden/books/" + testBook + "/../../laws/summary.md",
		"garden/tasks/../../laws/summary.md",
		"garden/tasks/irrigation/../private/prd.md",
		"garden/tasks/irrigation/" + filepath.Join(f.root, "laws", "summary.md"),
		"garden/books/../laws",
	}
	for _, p := range paths {
		if got, err := f.read(t, p); !errors.Is(err, ErrNotFound) {
			t.Errorf("read %q = %q, %v, want ErrNotFound", p, got, err)
		}
	}
}

func TestLawsAndHandoversInsideTheGrant(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.root, "laws", "water_daily.md"), "water daily\n")
	write(t, filepath.Join(f.root, "laws", "no_weeds.md"), "hidden")
	write(t, filepath.Join(f.root, "handover", "irrigation.md"), "resume here\n")
	write(t, filepath.Join(f.root, "handover", "whiteboard.md"), "hidden")
	write(t, filepath.Join(f.root, "handover", "archive", "compost.md"), "closed\n")
	f.grant.Shares = []Share{{
		Alias:     "garden",
		Laws:      []string{"water_daily", "summary"},
		Handovers: []string{"irrigation", "compost", "whiteboard"},
	}}

	got, err := f.read(t, "")
	if err != nil {
		t.Fatal(err)
	}
	want := "garden/laws/water_daily\ngarden/handovers/compost (archived)\ngarden/handovers/irrigation\n"
	if got != want {
		t.Errorf("contents = %q, want %q", got, want)
	}

	reads := map[string]string{
		"garden/laws":                 "garden/laws/water_daily\n",
		"garden/laws/water_daily":     "water daily\n",
		"garden/laws/water_daily.md":  "water daily\n",
		"garden/handovers/irrigation": "resume here\n",
		"garden/handovers/compost.md": "closed\n",
	}
	for path, want := range reads {
		if got, err := f.read(t, path); err != nil || got != want {
			t.Errorf("read %q = %q, %v, want %q", path, got, err, want)
		}
	}

	for _, path := range []string{
		"garden/laws/summary",
		"garden/laws/summary.md",
		"garden/laws/no_weeds",
		"garden/handovers/whiteboard",
		"garden/handovers/archive",
		"garden/handovers/archive/compost",
		"garden/laws/../handover/irrigation",
	} {
		if got, err := f.read(t, path); !errors.Is(err, ErrNotFound) {
			t.Errorf("read %q = %q, %v, want ErrNotFound", path, got, err)
		}
	}
}

func TestEmptyGrantSeesNothing(t *testing.T) {
	f := newFixture(t)
	f.grant.Shares = nil
	got, err := f.read(t, "")
	if err != nil || got != "Nothing is shared with this token.\n" {
		t.Errorf("contents = %q, %v", got, err)
	}
	for _, path := range []string{"garden", "garden/books/" + testBook, "garden/tasks/irrigation"} {
		if _, err := f.read(t, path); !errors.Is(err, ErrNotFound) {
			t.Errorf("read %q: %v, want ErrNotFound", path, err)
		}
	}
}

func TestSymlinkedEntriesAreRefused(t *testing.T) {
	f := newFixture(t)
	outside := filepath.Join(t.TempDir(), "outside.md")
	write(t, outside, "outside")
	if err := os.Symlink(outside, filepath.Join(f.root, "scratchpad", "irrigation", "link.md")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(f.root, "scratchpad", "linked")); err != nil {
		t.Fatal(err)
	}
	f.grant.Shares[0].Tasks = append(f.grant.Shares[0].Tasks, "linked")
	for _, p := range []string{"garden/tasks/irrigation/link.md", "garden/tasks/linked", "garden/tasks/linked/outside.md"} {
		if _, err := f.read(t, p); !errors.Is(err, ErrNotFound) {
			t.Errorf("read %q: err = %v, want ErrNotFound", p, err)
		}
	}
	got, _ := f.read(t, "garden/tasks/irrigation")
	if strings.Contains(got, "link.md") {
		t.Errorf("listing shows a symlink: %q", got)
	}
}

func TestTaskSubfolders(t *testing.T) {
	f := newFixture(t)
	task := filepath.Join(f.root, "scratchpad", "irrigation")
	write(t, filepath.Join(task, "feature", "handoff.md"), "contract\n")
	write(t, filepath.Join(task, "feature", "types.ts"), "export type A = string\n")
	write(t, filepath.Join(task, "feature", "snapshots", "shot.png"), string(pngBytes(t)))
	write(t, filepath.Join(task, "feature", "snapshots", "archive.zip"), "PK\x03\x04\x00\x00")
	write(t, filepath.Join(task, "feature", ".DS_Store"), "junk")
	write(t, filepath.Join(task, ".git", "HEAD"), "ref")
	write(t, filepath.Join(task, "a", "b", "c", "d", "deep.md"), "four down")
	write(t, filepath.Join(task, "a", "b", "c", "d", "e", "deeper.md"), "five down")

	got, err := f.read(t, "garden/tasks/irrigation/feature")
	if err != nil {
		t.Fatal(err)
	}
	if want := "handoff.md\ntypes.ts\nsnapshots/archive.zip (binary, not served)\nsnapshots/shot.png\n"; got != want {
		t.Errorf("feature listing = %q, want %q", got, want)
	}
	got, _ = f.read(t, "garden/tasks/irrigation")
	if !strings.Contains(got, "a/b/c/d/deep.md\n") || strings.Contains(got, "deeper") || strings.Contains(got, ".git") {
		t.Errorf("task listing ignores the depth limit or shows dot entries: %q", got)
	}
	for path, want := range map[string]string{
		"garden/tasks/irrigation/feature/handoff":  "contract\n",
		"garden/tasks/irrigation/feature/types.ts": "export type A = string\n",
		"garden/tasks/irrigation/a/b/c/d/deep.md":  "four down",
	} {
		if got, err := f.read(t, path); err != nil || got != want {
			t.Errorf("read %q = %q, %v, want %q", path, got, err, want)
		}
	}
	for _, p := range []string{
		"garden/tasks/irrigation/feature/.DS_Store",
		"garden/tasks/irrigation/.git/HEAD",
		"garden/tasks/irrigation/a/b/c/d/e",
		"garden/tasks/irrigation/a/b/c/d/e/deeper.md",
		"garden/tasks/irrigation/feature/handoff.md/x",
	} {
		if got, err := f.read(t, p); !errors.Is(err, ErrNotFound) {
			t.Errorf("read %q = %q, %v, want ErrNotFound", p, got, err)
		}
	}
}

func TestTaskImagesAndBinaries(t *testing.T) {
	f := newFixture(t)
	dir := filepath.Join(f.root, "scratchpad", "irrigation")
	shot := pngBytes(t)
	write(t, filepath.Join(dir, "shot.png"), string(shot))
	write(t, filepath.Join(dir, "huge.png"), string(shot[:8])+strings.Repeat("x", MaxImage))
	write(t, filepath.Join(dir, "archive.zip"), "PK\x03\x04\x00\x00")
	read := func(path string, offset int) (Content, error) {
		return f.lib.Read(f.cfg, f.grant, "garden/tasks/irrigation/"+path, offset)
	}
	c, err := read("shot.png", 0)
	if err != nil || !bytes.Equal(c.Image, shot) || c.MIME != "image/png" || c.Text != "" {
		t.Errorf("image read = %d bytes %q %q, %v", len(c.Image), c.MIME, c.Text, err)
	}
	if _, err := read("shot.png", 4); !errors.Is(err, ErrBadOffset) {
		t.Errorf("image with an offset: %v, want ErrBadOffset", err)
	}
	if _, err := read("huge.png", 0); !errors.Is(err, ErrImageTooLarge) {
		t.Errorf("image over the cap: %v, want ErrImageTooLarge", err)
	}
	if _, err := read("archive.zip", 0); !errors.Is(err, ErrNotServed) {
		t.Errorf("unsupported binary: %v, want ErrNotServed", err)
	}
}

func TestSymlinkedSubfolderIsRefused(t *testing.T) {
	f := newFixture(t)
	outside := t.TempDir()
	write(t, filepath.Join(outside, "secret.md"), "outside")
	if err := os.Symlink(outside, filepath.Join(f.root, "scratchpad", "irrigation", "linked")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{"garden/tasks/irrigation/linked", "garden/tasks/irrigation/linked/secret.md"} {
		if _, err := f.read(t, p); !errors.Is(err, ErrNotFound) {
			t.Errorf("read %q: err = %v, want ErrNotFound", p, err)
		}
	}
	if got, _ := f.read(t, "garden/tasks/irrigation"); strings.Contains(got, "linked") {
		t.Errorf("listing follows a symlinked folder: %q", got)
	}
}

func TestTaskListingIsCapped(t *testing.T) {
	f := newFixture(t)
	for i := range maxListing + 5 {
		write(t, filepath.Join(f.root, "scratchpad", "irrigation", "many", fmt.Sprintf("f%03d.md", i)), "x")
	}
	got, err := f.read(t, "garden/tasks/irrigation/many")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) != maxListing+1 || lines[maxListing] != "[listing cut: 5 more files not shown, read a subfolder to list it]" {
		t.Errorf("capped listing has %d lines ending %q", len(lines), lines[len(lines)-1])
	}
}

func TestDuplicateBookIdentityServesNeither(t *testing.T) {
	f := newFixture(t)
	write(t, filepath.Join(f.root, "books", "garden_design[2026-01-02][other]", "x[a].md"), "dup")
	if _, err := f.read(t, "garden/books/"+testBook); !errors.Is(err, ErrNotFound) {
		t.Errorf("duplicate identity read: err = %v, want ErrNotFound", err)
	}
	got, _ := f.read(t, "")
	if strings.Contains(got, "garden_design") {
		t.Errorf("contents lists a duplicated book: %q", got)
	}
}

func TestCutPagesThroughALongFile(t *testing.T) {
	line := strings.Repeat("a", 99) + "\n"
	data := []byte(strings.Repeat(line, 400))
	var got strings.Builder
	offset := 0
	for range 10 {
		part, err := cut(data, offset)
		if err != nil {
			t.Fatal(err)
		}
		body, trailer, found := strings.Cut(part, "\n[cut: ")
		got.WriteString(body)
		if !found {
			break
		}
		if len(body) > ReadCap {
			t.Fatalf("part of %d bytes exceeds the cap", len(body))
		}
		var shown, total int
		if _, err := fmt.Sscanf(trailer, "%d of %d bytes shown, read again with offset %d]", &shown, &total, &offset); err != nil {
			t.Fatalf("trailer %q: %v", trailer, err)
		}
	}
	if got.String() != string(data) {
		t.Errorf("paged read lost or repeated bytes: got %d, want %d", got.Len(), len(data))
	}
	if _, err := cut(data, len(data)+1); !errors.Is(err, ErrBadOffset) {
		t.Errorf("offset past the end: err = %v", err)
	}
}

func TestAuthenticate(t *testing.T) {
	token, hash, err := MintToken()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(token, "lgn_") || len(token) != 4+43 {
		t.Fatalf("token shape: %q", token)
	}
	now := time.Now()
	live := Grant{ID: "live", TokenHash: hash, ExpiresAt: now.Add(time.Hour)}
	cfg := Config{Grants: []Grant{live}}
	if g, ok := Authenticate(cfg, token, now); !ok || g.ID != "live" {
		t.Errorf("live token rejected")
	}
	if _, ok := Authenticate(cfg, token+"x", now); ok {
		t.Errorf("wrong token accepted")
	}
	cfg.Grants[0].Revoked = true
	if _, ok := Authenticate(cfg, token, now); ok {
		t.Errorf("revoked token accepted")
	}
	cfg.Grants[0].Revoked = false
	if _, ok := Authenticate(cfg, token, now.Add(2*time.Hour)); ok {
		t.Errorf("expired token accepted")
	}
}

func TestStoreReloadsOnChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.json")
	s := NewStore(path)
	cfg, err := s.Config()
	if err != nil || len(cfg.Grants) != 0 {
		t.Fatalf("missing file: %+v, %v", cfg, err)
	}
	write(t, path, `{"grants":[{"id":"a"}]}`)
	if cfg, _ := s.Config(); len(cfg.Grants) != 1 {
		t.Fatalf("new file not picked up: %+v", cfg)
	}
	write(t, path, `{"grants":[{"id":"a"},{"id":"b"}]}`)
	if cfg, _ := s.Config(); len(cfg.Grants) != 2 {
		t.Errorf("changed file not picked up: %+v", cfg)
	}
}

// The literal is what ui/src/main/node.ts writes: ISO times with milliseconds, a zero until, optional fields left out.
func TestReadsTheAppsNodeJSON(t *testing.T) {
	path := filepath.Join(t.TempDir(), "node.json")
	write(t, path, `{
  "projects": [{ "alias": "garden", "projectId": "p1" }],
  "grants": [{
    "id": "g1", "label": "Tester",
    "shares": [{ "alias": "garden", "books": ["garden_design[2026-01-02]"], "tasks": ["irrigation"] }],
    "expiresAt": "2026-10-14T10:00:00.123Z", "revoked": false, "createdAt": "2026-10-13T10:00:00.000Z",
    "tokenHash": "e163d0db2f3a5f9cd1c8d100c6c7b2e8e312fc593509a682cd4b4b1e5412a0b4"
  }],
  "tunnel": { "provider": "ngrok", "until": "0001-01-01T00:00:00Z" }
}`)
	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Tunnel.Until.IsZero() {
		t.Errorf("zero until parsed as %v", cfg.Tunnel.Until)
	}
	token := "lgn_BwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwcHBwc"
	before := time.Date(2026, 10, 14, 10, 0, 0, 0, time.UTC)
	if g, ok := Authenticate(cfg, token, before); !ok || g.ID != "g1" {
		t.Errorf("token minted by the app rejected")
	}
	if _, ok := Authenticate(cfg, token, before.Add(time.Second)); ok {
		t.Errorf("token accepted after its expiry")
	}
}
