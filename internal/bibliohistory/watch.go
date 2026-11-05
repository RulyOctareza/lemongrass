// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package bibliohistory

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/faizalv/lemongrass/internal/project"
	"github.com/fsnotify/fsnotify"
)

// Watcher commits every registered project's biblio folder a short while after it stops changing.
type Watcher struct {
	Projects func() ([]project.Project, error)
	GitDir   func(projectID string) string
	// Names who wrote a project's changed paths since a time; nil records no sessions.
	Sessions func(p project.Project, paths []string, since time.Time) []string
	Logf     func(format string, args ...any)

	Debounce time.Duration
	// The pass picks up new projects and folders, and commits whatever changed while nothing watched, such as edits
	// made while the app was closed.
	Pass time.Duration
	// History older than Keep is pruned once every PruneEvery, starting with the first pass; zero Keep prunes nothing.
	Keep       time.Duration
	PruneEvery time.Duration
}

type tracked struct {
	project project.Project
	root    string
	repo    *Repo
	timer   *time.Timer
}

func (w *Watcher) logf(format string, args ...any) {
	if w.Logf != nil {
		w.Logf(format, args...)
	}
}

func (w *Watcher) Run(ctx context.Context) error {
	fw, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer fw.Close()

	projects := map[string]*tracked{}
	due := make(chan string, 64)

	snapshot := func(t *tracked) {
		var sessions Sessions
		if w.Sessions != nil {
			sessions = func(paths []string, since time.Time) []string { return w.Sessions(t.project, paths, since) }
		}
		if _, err := t.repo.Snapshot(sessions); err != nil {
			w.logf("%v (%s)", err, t.project.Path)
		}
	}

	var lastPrune time.Time
	gitMissing := false
	pass := func() {
		if _, err := GitPath(); err != nil {
			if !gitMissing {
				w.logf("%v", err)
				gitMissing = true
			}
			return
		}
		gitMissing = false
		defer func() {
			if w.Keep <= 0 || time.Since(lastPrune) < w.PruneEvery {
				return
			}
			lastPrune = time.Now()
			for _, t := range projects {
				if _, err := t.repo.Prune(time.Now().Add(-w.Keep)); err != nil {
					w.logf("%v (%s)", err, t.project.Path)
				}
			}
		}()
		list, err := w.Projects()
		if err != nil {
			w.logf("bibliohistory: projects: %v", err)
		} else {
			dropGone(fw, projects, list)
		}
		for _, p := range list {
			root := filepath.Join(p.Path, "biblio")
			if info, err := os.Stat(root); err != nil || !info.IsDir() {
				continue
			}
			t, ok := projects[p.ID]
			if !ok {
				repo := New(w.GitDir(p.ID), root)
				if err := repo.Init(); err != nil {
					w.logf("%v (%s)", err, p.Path)
					continue
				}
				t = &tracked{project: p, root: root, repo: repo}
				projects[p.ID] = t
			}
			watchRoot(fw, root)
			snapshot(t)
		}
	}

	owner := func(path string) *tracked {
		for _, t := range projects {
			if path == t.root || strings.HasPrefix(path, t.root+string(filepath.Separator)) {
				return t
			}
		}
		return nil
	}

	pass()
	tick := time.NewTicker(w.Pass)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			for _, t := range projects {
				if t.timer != nil {
					t.timer.Stop()
				}
			}
			return nil
		case ev, ok := <-fw.Events:
			if !ok {
				return nil
			}
			if ev.Op == fsnotify.Chmod {
				continue
			}
			t := owner(ev.Name)
			if t == nil {
				continue
			}
			if ev.Op.Has(fsnotify.Create) {
				if info, err := os.Stat(ev.Name); err == nil && info.IsDir() {
					watchNew(fw, ev.Name)
				}
			}
			id := t.project.ID
			if t.timer == nil {
				t.timer = time.AfterFunc(w.Debounce, func() {
					select {
					case due <- id:
					case <-ctx.Done():
					}
				})
			} else {
				t.timer.Reset(w.Debounce)
			}
		case err, ok := <-fw.Errors:
			if !ok {
				return nil
			}
			w.logf("bibliohistory: watcher: %v", err)
		case id := <-due:
			if t := projects[id]; t != nil {
				snapshot(t)
			}
		case <-tick.C:
			pass()
		}
	}
}

// dropGone stops watching projects that left the registry, so their folders, which purge removes, hold no watch.
func dropGone(fw *fsnotify.Watcher, projects map[string]*tracked, list []project.Project) {
	for id, t := range projects {
		if slices.ContainsFunc(list, func(p project.Project) bool { return p.ID == id }) {
			continue
		}
		if t.timer != nil {
			t.timer.Stop()
		}
		for _, watched := range fw.WatchList() {
			if watched == t.root || strings.HasPrefix(watched, t.root+string(filepath.Separator)) {
				fw.Remove(watched)
			}
		}
		delete(projects, id)
	}
}
