// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package bibliohistory

import (
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/session"
)

// Attribute returns the sessions whose logged edits hit any of changed, each once, in the order they first touched one.
// The hook logs the path the tool was given, which may be relative to the project or go through a symlink, so both
// the logged path and the biblio folder are compared in plain and symlink-resolved form.
func Attribute(projectPath string, touches []session.FileTouch, changed []string) []string {
	want := map[string]bool{}
	for _, root := range biblioRoots(projectPath) {
		for _, p := range changed {
			want[filepath.Join(root, filepath.FromSlash(p))] = true
		}
	}

	seen := map[string]bool{}
	var who []string
	for _, t := range touches {
		if seen[t.SessionID] {
			continue
		}
		p := t.FilePath
		if !filepath.IsAbs(p) {
			p = filepath.Join(projectPath, p)
		}
		p = filepath.Clean(p)
		hit := want[p]
		if !hit {
			if dir, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
				hit = want[filepath.Join(dir, filepath.Base(p))]
			}
		}
		if hit {
			seen[t.SessionID] = true
			who = append(who, t.SessionID)
		}
	}
	return who
}
