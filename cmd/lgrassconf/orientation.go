package main

import (
	"embed"
	"io/fs"
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/bibliothek"
)

//go:embed orientation
var orientationFS embed.FS

func (k *keeper) orientationDir() string {
	return filepath.Join(k.home, ".lemongrass", "orientation")
}

const bibliothekHeading = "\n# The bibliothek convention\n\n"

// Each installed orientation is followed by bibliothek's text, so a tab never loads the skill.
func (k *keeper) loadOrientation() []skillFile {
	entries, err := fs.ReadDir(orientationFS, "orientation")
	if err != nil {
		panic(err)
	}
	convention, err := bibliothek.Prompt()
	if err != nil {
		panic(err)
	}
	var files []skillFile
	for _, e := range entries {
		data, err := orientationFS.ReadFile("orientation/" + e.Name())
		if err != nil {
			panic(err)
		}
		content := append(append(data, bibliothekHeading...), convention...)
		files = append(files, skillFile{root: k.orientationDir(), path: filepath.Join(k.orientationDir(), e.Name()), content: content})
	}
	return files
}

func (k *keeper) managedFiles() []skillFile {
	files := make([]skillFile, 0, len(k.skills)+len(k.orientation)+len(k.plugin))
	return append(append(append(files, k.skills...), k.orientation...), k.plugin...)
}
