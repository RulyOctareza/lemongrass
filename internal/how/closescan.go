// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package how

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// Every scan is capped so the close's output grows with the task, never with the project.
const (
	staleHitsLimit  = 15
	quotedLineLimit = 10
)

var (
	scopeLine      = regexp.MustCompile(`(?i)out of scope|not in scope|\bdeferred\b|\bfollow-?up\b|\bfor later\b|\blater task\b`)
	correctionLine = regexp.MustCompile(`(?i)\bcorrect(ed|ion|ions)?\b`)
	questionLine   = regexp.MustCompile(`(?i)\bTBD\b|open question|\bundecided\b`)
	rootCause      = regexp.MustCompile(`(?i)root\s+cause`)
)

// staleRefs finds references to the slug in books, laws, handovers and the project's markdown, before the archive
// moves the task. Other tasks are left to relatedTasks. The task's own handover and the whiteboard are left out when
// the handover step already rewrites them.
func staleRefs(root, slug, handover string) []string {
	word := slugWord(slug)
	biblio := filepath.Join(root, "biblio")
	skip := map[string]bool{}
	if handover != "" {
		skip[filepath.Join(biblio, "handover", handover)] = true
		skip[filepath.Join(biblio, "handover", "whiteboard.md")] = true
	}
	var files []string
	for _, dir := range []string{"books", "laws", "handover"} {
		top := filepath.Join(biblio, dir)
		filepath.WalkDir(top, func(p string, d fs.DirEntry, err error) error {
			if err != nil || skip[p] {
				if d != nil && d.IsDir() {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				if d.Name() == "archive" && filepath.Dir(p) == top {
					return fs.SkipDir
				}
				return nil
			}
			if filepath.Ext(p) == ".md" {
				files = append(files, p)
			}
			return nil
		})
	}
	if out, err := gitOut(root, "ls-files", "-z", "--", "*.md"); err == nil {
		for _, rel := range strings.Split(string(out), "\x00") {
			if rel != "" {
				files = append(files, filepath.Join(root, filepath.FromSlash(rel)))
			}
		}
	}
	var hits []string
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		rel, _ := filepath.Rel(root, p)
		for i, line := range strings.Split(string(data), "\n") {
			if word.MatchString(line) {
				hits = append(hits, fmt.Sprintf("`%s:%d`: %s", filepath.ToSlash(rel), i+1, clipRunes(strings.TrimSpace(line), 100)))
				if len(hits) == staleHitsLimit {
					return append(hits, "more references exist; search for the slug to find the rest")
				}
			}
		}
	}
	return hits
}

// matchingLines quotes the lines of text that match re, trimmed and capped.
func matchingLines(text string, re *regexp.Regexp) []string {
	var out []string
	for _, line := range strings.Split(text, "\n") {
		if !re.MatchString(line) {
			continue
		}
		out = append(out, clipRunes(strings.TrimSpace(line), 160))
		if len(out) == quotedLineLimit {
			break
		}
	}
	return out
}

func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) > n {
		return string(r[:n]) + "..."
	}
	return s
}
