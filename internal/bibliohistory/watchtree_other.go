// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package bibliohistory

import (
	"io/fs"
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// fsnotify watches one folder at a time here, so a tree is watched by adding each of its folders; adding one already
// watched is harmless.
func watchRoot(fw *fsnotify.Watcher, root string) {
	addTree(fw, root)
}

func watchNew(fw *fsnotify.Watcher, dir string) {
	addTree(fw, dir)
}

func addTree(fw *fsnotify.Watcher, root string) {
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err == nil && d.IsDir() {
			fw.Add(p)
		}
		return nil
	})
}
