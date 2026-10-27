// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package agentconf

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var inlineProjects = regexp.MustCompile(`(?m)^\s*projects\s*=`)

// TrustCodexProject appends a trusted [projects."<path>"] table to Codex's config.toml unless the file already names that path, so an earlier choice is never overridden. A missing Codex directory is skipped.
func TrustCodexProject(configPath, projectPath string) error {
	if _, err := os.Stat(filepath.Dir(configPath)); errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	mode := os.FileMode(0o600)
	data, err := os.ReadFile(configPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		data = nil
	case err != nil:
		return err
	default:
		if info, err := os.Stat(configPath); err == nil {
			mode = info.Mode().Perm()
		}
	}
	text := string(data)
	quoted := `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(projectPath) + `"`
	if strings.Contains(text, quoted) || strings.Contains(text, "'"+projectPath+"'") {
		return nil
	}
	if inlineProjects.MatchString(text) {
		return fmt.Errorf("agentconf: %s defines projects inline, so it was left unchanged", configPath)
	}
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	text += "\n[projects." + quoted + "]\ntrust_level = \"trusted\"\n"
	return WriteAtomic(configPath, []byte(text), mode)
}
