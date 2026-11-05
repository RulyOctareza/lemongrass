// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// A footprint above lightFilesLimit files, or with a new file, sends its task to the full close; the close lists at
// most footprintListLimit of them.
const (
	lightFilesLimit    = 8
	footprintListLimit = 30
)

var namedPath = regexp.MustCompile(`[A-Za-z0-9_.\-]+(?:/[A-Za-z0-9_.\-]+)+`)

type footFile struct {
	path  string // relative to the project root, slash-separated
	state string // "new", "uncommitted", "committed", or "" when git could not tell
	// edited is true when a session that wrote the task's documents edited the file, false when the documents only
	// name it.
	edited bool
}

type footprint struct {
	files []footFile
}

func (f footprint) newFiles() int {
	n := 0
	for _, file := range f.files {
		if file.state == "new" {
			n++
		}
	}
	return n
}

func (f footprint) paths() []string {
	out := make([]string, len(f.files))
	for i, file := range f.files {
		out[i] = file.path
	}
	return out
}

// taskFootprint is the code the task changed, from two witnesses. The edit log gives every edit, outside biblio, by a
// session that also wrote the task's documents since the task started; a session that worked several tasks brings
// their files too. The documents give the existing files they name, which is all there is for a task older than the
// log or built through the shell. The close lists the two apart.
func taskFootprint(env Env, slug string, age taskAge) footprint {
	var fp footprint
	seen := map[string]bool{}
	states := gitStates(env.Root, age)
	add := func(paths []string, edited bool) {
		for _, p := range paths {
			if seen[p] {
				continue
			}
			seen[p] = true
			state := states[p]
			if state == "" && states != nil {
				state = "committed"
			}
			fp.files = append(fp.files, footFile{path: p, state: state, edited: edited})
		}
	}
	if env.EditsSince != nil {
		add(editedPaths(env, slug, age), true)
	}
	add(namedPaths(env.Root, slug), false)
	return fp
}

func editedPaths(env Env, slug string, age taskAge) []string {
	// The biblio history records a task a moment after its first file is written, so the window opens earlier to keep
	// the session that created it.
	edits := env.EditsSince(age.t.Add(-time.Hour))
	roots := resolvedForms(env.Root)
	taskDirs := resolvedForms(filepath.Join(env.Root, "biblio", "scratchpad", slug))
	sessions := map[string]bool{}
	for _, e := range edits {
		if _, ok := relUnder(absEdit(env.Root, e.Path), taskDirs); ok {
			sessions[e.Session] = true
		}
	}
	if len(sessions) == 0 {
		return nil
	}
	seen := map[string]bool{}
	var paths []string
	for _, e := range edits {
		if !sessions[e.Session] {
			continue
		}
		rel, ok := relUnder(absEdit(env.Root, e.Path), roots)
		if !ok || rel == "biblio" || strings.HasPrefix(rel, "biblio/") || seen[rel] {
			continue
		}
		seen[rel] = true
		paths = append(paths, rel)
	}
	sort.Strings(paths)
	return paths
}

func absEdit(root, p string) string {
	if !filepath.IsAbs(p) {
		p = filepath.Join(root, p)
	}
	return filepath.Clean(p)
}

// The hook logs the path the tool was given, which may run through a symlink, so a directory is matched in both its
// plain and resolved form.
func resolvedForms(dir string) []string {
	forms := []string{filepath.Clean(dir)}
	if r, err := filepath.EvalSymlinks(dir); err == nil && r != forms[0] {
		forms = append(forms, r)
	}
	return forms
}

func relUnder(p string, dirs []string) (string, bool) {
	cands := []string{p}
	if d, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		cands = append(cands, filepath.Join(d, filepath.Base(p)))
	}
	for _, c := range cands {
		for _, d := range dirs {
			if rel, err := filepath.Rel(d, c); err == nil && rel != "." && !strings.HasPrefix(rel, "..") {
				return filepath.ToSlash(rel), true
			}
		}
	}
	return "", false
}

func namedPaths(root, slug string) []string {
	dir := filepath.Join(root, "biblio", "scratchpad", slug)
	text := readText(filepath.Join(dir, "prd.md")) + "\n" + readText(filepath.Join(dir, "activities.md"))
	seen := map[string]bool{}
	var paths []string
	for _, m := range namedPath.FindAllString(text, -1) {
		p := strings.TrimRight(m, ".")
		if seen[p] || strings.HasPrefix(p, "biblio/") || strings.Contains(p, "scratchpad/") {
			continue
		}
		seen[p] = true
		if info, err := os.Stat(filepath.Join(root, filepath.FromSlash(p))); err == nil && !info.IsDir() {
			paths = append(paths, p)
		}
	}
	sort.Strings(paths)
	return paths
}

// gitStates maps changed paths to "new" or "uncommitted"; a path it leaves out is committed or unchanged. Files added
// in a commit since the task started count as new. It is nil when the project's git cannot be read.
func gitStates(root string, age taskAge) map[string]string {
	prefixOut, err := gitOut(root, "rev-parse", "--show-prefix")
	if err != nil {
		return nil
	}
	prefix := strings.TrimSpace(string(prefixOut))
	out, err := gitOut(root, "status", "--porcelain=v1", "-z", "--untracked-files=all", "--", ".")
	if err != nil {
		return nil
	}
	states := map[string]string{}
	fields := strings.Split(string(out), "\x00")
	for i := 0; i < len(fields); i++ {
		f := fields[i]
		if len(f) < 4 {
			continue
		}
		code, path := f[:2], strings.TrimPrefix(f[3:], prefix)
		if code[0] == 'R' || code[0] == 'C' {
			i++ // the old path follows a rename or copy
		}
		if code == "??" || code[0] == 'A' {
			states[path] = "new"
		} else {
			states[path] = "uncommitted"
		}
	}
	if age.known {
		since := age.t.Format(time.RFC3339)
		if added, err := gitOut(root, "log", "--since="+since, "--diff-filter=A", "--name-only", "--relative", "--format=", "--", "."); err == nil {
			for _, p := range strings.Split(string(added), "\n") {
				if p = strings.TrimSpace(p); p != "" {
					states[p] = "new"
				}
			}
		}
	}
	return states
}

func gitOut(root string, args ...string) ([]byte, error) {
	cmd := exec.Command("git", append([]string{"-C", root, "-c", "core.quotePath=false"}, args...)...)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	if err := cmd.Run(); err != nil {
		return nil, err
	}
	return stdout.Bytes(), nil
}

type relatedChapter struct {
	path  string // relative to the project root
	terms []string
	score float64
}

const relatedChaptersLimit = 8

// relatedChapters ranks the chapters that name footprint files, by the file's path, its base name when distinctive,
// or its own folder. Each file credits a chapter with its most specific term, weighted by how rare that term is across
// the books, so a file named in one chapter ranks it first. A folder named in more than two chapters and in more than
// one in eight points nowhere in particular and is not counted.
func relatedChapters(root string, files []string) []relatedChapter {
	if len(files) == 0 {
		return nil
	}
	books := filepath.Join(root, "biblio", "books")
	type chapter struct{ rel, text string }
	var chapters []chapter
	filepath.WalkDir(books, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || filepath.Ext(p) != ".md" || filepath.Dir(p) == books {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		chapters = append(chapters, chapter{filepath.ToSlash(rel), readText(p)})
		return nil
	})
	df := map[string]int{}
	termSets := make([][]weightedTerm, len(files))
	for i, f := range files {
		termSets[i] = fileTerms(f)
		for _, t := range termSets[i] {
			if _, done := df[t.term]; done {
				continue
			}
			for _, c := range chapters {
				if containsTerm(c.text, t.term) {
					df[t.term]++
				}
			}
		}
	}
	for i, set := range termSets {
		var kept []weightedTerm
		for _, t := range set {
			if t.weight > 1 || df[t.term] <= 2 || df[t.term]*8 <= len(chapters) {
				kept = append(kept, t)
			}
		}
		termSets[i] = kept
	}
	var found []relatedChapter
	for _, c := range chapters {
		r := relatedChapter{path: c.rel}
		for _, set := range termSets {
			for _, t := range set {
				if containsTerm(c.text, t.term) {
					r.terms = append(r.terms, t.term)
					r.score += t.weight / float64(df[t.term])
					break
				}
			}
		}
		if len(r.terms) > 0 {
			found = append(found, r)
		}
	}
	sort.SliceStable(found, func(i, j int) bool {
		if found[i].score != found[j].score {
			return found[i].score > found[j].score
		}
		return found[i].path < found[j].path
	})
	if len(found) > relatedChaptersLimit {
		found = found[:relatedChaptersLimit]
	}
	return found
}

type weightedTerm struct {
	term   string
	weight float64
}

// Most specific first. Only the file's own folder counts, since a folder further up names too much to point anywhere.
func fileTerms(path string) []weightedTerm {
	terms := []weightedTerm{{path, 3}}
	base := filepath.Base(path)
	if len(strings.TrimSuffix(base, filepath.Ext(base))) >= 6 {
		terms = append(terms, weightedTerm{base, 2})
	}
	if dir := filepath.ToSlash(filepath.Dir(path)); strings.Contains(dir, "/") {
		terms = append(terms, weightedTerm{dir, 1})
	}
	return terms
}

func (c relatedChapter) String() string {
	terms := uniq(c.terms)
	if len(terms) > 3 {
		terms = append(terms[:3], "...")
	}
	return fmt.Sprintf("`%s` (names %s)", c.path, strings.Join(terms, ", "))
}

func uniq(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// containsTerm matches term only where it is not the start of a longer name, so `cmd/lgrass` does not match
// `cmd/lgrassd`.
func containsTerm(text, term string) bool {
	for i := 0; ; {
		j := strings.Index(text[i:], term)
		if j < 0 {
			return false
		}
		end := i + j + len(term)
		if end == len(text) || !nameChar(text[end]) {
			return true
		}
		i += j + 1
	}
}

func nameChar(c byte) bool {
	return c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z'
}
