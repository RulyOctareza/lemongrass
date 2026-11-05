// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package bibliohistory

import (
	"path/filepath"

	"github.com/fsnotify/fsnotify"
)

// Every watched folder is an open directory handle on Windows, and moving a folder that holds open handles below it,
// such as archiving a scratchpad task, can fail. One recursive watch on biblio holds a single handle above every
// folder a model moves and sees new folders without being told.
func watchRoot(fw *fsnotify.Watcher, root string) {
	fw.Add(filepath.Join(root, "..."))
}

func watchNew(fw *fsnotify.Watcher, dir string) {}
