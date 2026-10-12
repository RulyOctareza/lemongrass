package main

import (
	"embed"
	"io/fs"
	"path/filepath"
	"strings"
)

//go:embed all:plugin
var pluginFS embed.FS

func (k *keeper) pluginDir() string {
	return filepath.Join(k.home, ".lemongrass", "plugin")
}

func (k *keeper) loadPlugin() []skillFile {
	var files []skillFile
	root := k.pluginDir()
	err := fs.WalkDir(pluginFS, "plugin", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		data, err := pluginFS.ReadFile(p)
		if err != nil {
			return err
		}
		rel := strings.TrimPrefix(p, "plugin/")
		files = append(files, skillFile{root: root, path: filepath.Join(root, filepath.FromSlash(rel)), content: data})
		return nil
	})
	if err != nil {
		panic(err)
	}
	return files
}

func (k *keeper) pluginDirs() []string {
	seen := map[string]bool{k.pluginDir(): true}
	dirs := []string{k.pluginDir()}
	for _, f := range k.plugin {
		for dir := filepath.Dir(f.path); dir != f.root; dir = filepath.Dir(dir) {
			if !seen[dir] {
				seen[dir] = true
				dirs = append(dirs, dir)
			}
		}
	}
	return dirs
}
