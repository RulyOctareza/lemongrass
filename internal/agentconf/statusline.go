// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Ownership is matched on this fragment, not the full command, so a statusline written with an older lgrassd path is
// still recognized as Lemongrass's and rewritten instead of being adopted as the human's own.
const statusLineFragment = " statusline --account "

func StatusLineCommand(lgrassdPath, account string) string {
	return lgrassdPath + statusLineFragment + account
}

func ownedStatusLine(command string) bool {
	return strings.Contains(command, statusLineFragment)
}

// ReconcileStatusLine puts ours into statusLine.command and returns the human's command the store must hold afterwards.
// An empty ours means the lgrassd binary is absent, and then the saved human command is put back. Every other key of
// the statusLine object stays as the human set it.
func ReconcileStatusLine(data []byte, ours, saved string) ([]byte, bool, string, error) {
	top := map[string]json.RawMessage{}
	if len(bytes.TrimSpace(data)) > 0 {
		if err := json.Unmarshal(data, &top); err != nil {
			return nil, false, "", fmt.Errorf("agentconf: parsing settings: %w", err)
		}
	}
	line := map[string]json.RawMessage{}
	if raw, ok := top["statusLine"]; ok && string(bytes.TrimSpace(raw)) != "null" {
		if err := json.Unmarshal(raw, &line); err != nil {
			return nil, false, "", fmt.Errorf("agentconf: parsing statusLine: %w", err)
		}
	}
	var current string
	if raw, ok := line["command"]; ok {
		json.Unmarshal(raw, &current)
	}
	owned := ownedStatusLine(current)

	var want, keep string
	switch {
	case ours != "" && owned:
		want, keep = ours, saved
	case ours != "":
		want, keep = ours, current
	case owned:
		want, keep = saved, ""
	default:
		return data, false, "", nil
	}
	if want == current {
		return data, false, keep, nil
	}

	if want == "" {
		delete(top, "statusLine")
	} else {
		line["type"] = json.RawMessage(`"command"`)
		command, err := json.Marshal(want)
		if err != nil {
			return nil, false, "", err
		}
		line["command"] = command
		lineJSON, err := json.Marshal(line)
		if err != nil {
			return nil, false, "", err
		}
		top["statusLine"] = lineJSON
	}
	out, err := json.MarshalIndent(top, "", "  ")
	if err != nil {
		return nil, false, "", err
	}
	return append(out, '\n'), true, keep, nil
}

// StatusLineStore holds the human's own statusline command per Claude account, which `lgrassd statusline` runs after
// lgrassconf has taken the statusLine slot. Only lgrassconf and an account add write it.
type StatusLineStore struct {
	Path string
}

type statusLineFile struct {
	Commands map[string]string `json:"commands"`
}

func StatusLineStorePath(home string) string {
	return filepath.Join(home, ".lemongrass", "statusline.json")
}

func (s StatusLineStore) load() (map[string]string, error) {
	data, err := os.ReadFile(s.Path)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var file statusLineFile
	if err := json.Unmarshal(data, &file); err != nil {
		return nil, fmt.Errorf("agentconf: parsing %s: %w", s.Path, err)
	}
	if file.Commands == nil {
		file.Commands = map[string]string{}
	}
	return file.Commands, nil
}

func (s StatusLineStore) save(commands map[string]string) error {
	data, err := json.MarshalIndent(statusLineFile{Commands: commands}, "", "  ")
	if err != nil {
		return err
	}
	return WriteAtomic(s.Path, append(data, '\n'), 0o600)
}

func (s StatusLineStore) Command(account string) (string, error) {
	commands, err := s.load()
	if err != nil {
		return "", err
	}
	return commands[account], nil
}

func (s StatusLineStore) set(account, command string) error {
	commands, err := s.load()
	if err != nil {
		return err
	}
	if commands[account] == command {
		return nil
	}
	if command == "" {
		delete(commands, account)
	} else {
		commands[account] = command
	}
	return s.save(commands)
}

// Prune drops the entries of accounts that no longer exist.
func (s StatusLineStore) Prune(accounts []string) error {
	commands, err := s.load()
	if err != nil {
		return err
	}
	live := map[string]bool{}
	for _, a := range accounts {
		live[a] = true
	}
	var gone []string
	for a := range commands {
		if !live[a] {
			gone = append(gone, a)
		}
	}
	if len(gone) == 0 {
		return nil
	}
	sort.Strings(gone)
	for _, a := range gone {
		delete(commands, a)
	}
	return s.save(commands)
}

// SyncStatusLine reconciles one settings file. A command to keep is stored before the settings are written and a
// dropped one is forgotten only after, so a crash between the two never loses the human's own command.
func SyncStatusLine(settingsPath, lgrassdPath, account string, store StatusLineStore) error {
	saved, err := store.Command(account)
	if err != nil {
		return err
	}
	ours := ""
	if lgrassdPath != "" {
		ours = StatusLineCommand(lgrassdPath, account)
	}
	keep := saved
	reconcile := func(data []byte, _ string) ([]byte, bool, error) {
		out, changed, k, err := ReconcileStatusLine(data, ours, saved)
		if err != nil {
			return nil, false, err
		}
		keep = k
		if keep != "" && keep != saved {
			if err := store.set(account, keep); err != nil {
				return nil, false, err
			}
		}
		return out, changed, nil
	}
	if err := SyncHookConfig(settingsPath, lgrassdPath, reconcile); err != nil {
		return err
	}
	if keep == "" && saved != "" {
		return store.set(account, "")
	}
	return nil
}
