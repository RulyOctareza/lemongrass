// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package project resolves a directory to its registered lemongrass project.
package project

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/config"
	"github.com/google/uuid"
)

// Project mirrors ui/src/main/projects.ts's shape in
// ~/.lemongrass/projects.json.
type Project struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

func registryPath() string {
	return filepath.Join(config.Dir(), "projects.json")
}

func loadAll() ([]Project, error) {
	data, err := os.ReadFile(registryPath())
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var projects []Project
	if err := json.Unmarshal(data, &projects); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", registryPath(), err)
	}
	return projects, nil
}

// List returns every registered project, or none when the registry does not exist yet.
func List() ([]Project, error) {
	return loadAll()
}

func saveAll(projects []Project) error {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(projects, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(registryPath(), data, 0o644)
}

// Register adds dir to the project registry, deduped by symlink-resolved path, returning the existing entry unchanged if already registered.
func Register(dir string) (Project, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, err
	}
	abs = resolvePath(abs)

	projects, err := loadAll()
	if err != nil {
		return Project{}, err
	}
	for _, p := range projects {
		if resolvePath(p.Path) == abs {
			return p, nil
		}
	}

	p := Project{ID: uuid.NewString(), Name: filepath.Base(abs), Path: abs}
	if err := saveAll(append(projects, p)); err != nil {
		return Project{}, err
	}
	return p, nil
}

// resolvePath cleans dir and resolves symlinks, falling back to the cleaned path if resolution fails.
func resolvePath(dir string) string {
	clean := filepath.Clean(dir)
	if resolved, err := filepath.EvalSymlinks(clean); err == nil {
		return resolved
	}
	return clean
}

// Resolve matches a directory to a registered project, both symlink-resolved: exact path first, then the nearest registered ancestor.
func Resolve(dir string) (Project, error) {
	projects, err := loadAll()
	if err != nil {
		return Project{}, err
	}
	if len(projects) == 0 {
		return Project{}, fmt.Errorf("no projects registered in %s", registryPath())
	}

	abs, err := filepath.Abs(dir)
	if err != nil {
		return Project{}, err
	}
	abs = resolvePath(abs)

	resolvedPaths := make([]string, len(projects))
	for i, p := range projects {
		resolvedPaths[i] = resolvePath(p.Path)
	}

	for {
		for i, p := range projects {
			if resolvedPaths[i] == abs {
				return p, nil
			}
		}
		parent := filepath.Dir(abs)
		if parent == abs {
			break
		}
		abs = parent
	}

	return Project{}, fmt.Errorf("%s is not inside a registered lemongrass project", dir)
}

// ResolveCwd resolves the current working directory to its project.
func ResolveCwd() (Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return Project{}, err
	}
	return Resolve(cwd)
}
