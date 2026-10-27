// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package claudeaccount

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/faizalv/lemongrass/internal/agentconf"
)

// ClaudeJSONPath is where Claude Code keeps project trust: ~/.claude.json for the primary (name ""), and inside the config directory for an account.
func (a Accounts) ClaudeJSONPath(name string) string {
	if name == "" {
		return filepath.Join(a.Home, ".claude.json")
	}
	return filepath.Join(a.Dir(name), ".claude.json")
}

// Trust marks projectPath trusted for one account, name "" being the primary. A missing file is skipped because Claude Code creates it at the first login.
func (a Accounts) Trust(name, projectPath string) error {
	path := a.ClaudeJSONPath(name)
	info, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(data, &root); err != nil || root == nil {
		return fmt.Errorf("claudeaccount: %s is not a JSON object", path)
	}
	projects := map[string]json.RawMessage{}
	if raw, ok := root["projects"]; ok {
		if err := json.Unmarshal(raw, &projects); err != nil || projects == nil {
			return fmt.Errorf("claudeaccount: projects in %s is not a JSON object", path)
		}
	}
	entry := map[string]json.RawMessage{}
	if raw, ok := projects[projectPath]; ok {
		if err := json.Unmarshal(raw, &entry); err != nil || entry == nil {
			return fmt.Errorf("claudeaccount: the entry for %s in %s is not a JSON object", projectPath, path)
		}
	}
	var accepted bool
	if raw, ok := entry["hasTrustDialogAccepted"]; ok && json.Unmarshal(raw, &accepted) == nil && accepted {
		return nil
	}
	entry["hasTrustDialogAccepted"] = json.RawMessage("true")
	if projects[projectPath], err = json.Marshal(entry); err != nil {
		return err
	}
	if root["projects"], err = json.Marshal(projects); err != nil {
		return err
	}
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	return agentconf.WriteAtomic(path, append(out, '\n'), info.Mode().Perm())
}

// TrustAll marks projectPath trusted for the primary and every listed account, trying each even when one fails.
func (a Accounts) TrustAll(projectPath string) error {
	names, err := a.Names()
	if err != nil {
		return err
	}
	errs := []error{a.Trust("", projectPath)}
	for _, name := range names {
		errs = append(errs, a.Trust(name, projectPath))
	}
	return errors.Join(errs...)
}
