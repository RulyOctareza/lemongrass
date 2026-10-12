package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/fsnotify/fsnotify"
)

const (
	debounce      = 200 * time.Millisecond
	safetyNetTick = 10 * time.Minute
)

func (k *keeper) skillDirs() []string {
	seen := map[string]bool{}
	var dirs []string
	add := func(dir string) {
		if !seen[dir] {
			seen[dir] = true
			dirs = append(dirs, dir)
		}
	}
	for _, v := range vendors {
		add(filepath.Join(k.home, v.configDir))
		add(k.skillsRoot(v))
		for _, name := range legacySkillNames {
			add(k.legacySkillDir(v, name))
		}
	}
	for _, f := range k.skills {
		for dir := filepath.Dir(f.path); dir != f.root; dir = filepath.Dir(dir) {
			add(dir)
		}
	}
	return dirs
}

func (k *keeper) watchedDirs() []string {
	dirs := append(append(k.skillDirs(), k.orientationDir()), k.pluginDirs()...)
	for _, c := range k.candidates {
		dirs = append(dirs, filepath.Dir(c))
	}
	accounts := k.accounts()
	dirs = append(dirs, filepath.Dir(accounts.FilePath()))
	names, _ := accounts.Names()
	for _, name := range names {
		dirs = append(dirs, accounts.Dir(name))
	}
	return dirs
}

func (k *keeper) relevant() map[string]bool {
	set := map[string]bool{
		k.claudeSettingsPath(): true,
		k.codexHooksPath():     true,
	}
	for _, dir := range k.skillDirs() {
		set[dir] = true
	}
	set[k.orientationDir()] = true
	for _, dir := range k.pluginDirs() {
		set[dir] = true
	}
	for _, f := range k.managedFiles() {
		set[f.path] = true
	}
	for _, v := range vendors {
		for _, name := range legacySkillNames {
			set[filepath.Join(k.legacySkillDir(v, name), "SKILL.md")] = true
		}
	}
	for _, c := range k.candidates {
		set[c] = true
	}
	accounts := k.accounts()
	set[accounts.FilePath()] = true
	names, _ := accounts.Names()
	for _, name := range names {
		set[accounts.SettingsPath(name)] = true
	}
	return set
}

func (k *keeper) addWatches(w *fsnotify.Watcher) {
	for _, dir := range k.watchedDirs() {
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			w.Add(dir)
		}
	}
}

func (k *keeper) run(ctx context.Context) error {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return err
	}
	defer w.Close()

	var relevant map[string]bool
	pass := func() {
		if err := k.reconcile(); err != nil {
			log.Printf("reconcile: %v", err)
		}
		k.addWatches(w)
		relevant = k.relevant()
	}
	pass()

	timer := time.NewTimer(debounce)
	timer.Stop()
	tick := time.NewTicker(safetyNetTick)
	defer tick.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case ev := <-w.Events:
			if relevant[ev.Name] {
				timer.Reset(debounce)
			}
		case err := <-w.Errors:
			log.Printf("watcher: %v", err)
			timer.Reset(debounce)
		case <-timer.C:
			pass()
		case <-tick.C:
			pass()
		}
	}
}
