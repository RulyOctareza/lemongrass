// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package guard decides whether an agent tool call is allowed, denied, or needs the user's approval, from a built-in catalog of dangerous operations and the user's own policy.
package guard

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

type Mode int

const (
	ModeDeny Mode = iota
	ModeApproval
)

const (
	overrideAllow    = "allow"
	overrideDeny     = "deny"
	overrideApproval = "approval"
)

const (
	maxPolicyRules    = 200
	maxPolicyBinaries = 200
	maxPolicyPaths    = 200
)

// Policy is the user's changes on top of the built-in catalog. Stored, it is one scope: the global policy uses only Binaries and Paths, and a project's may also exclude global entries. Combine turns the two into the one Decide enforces.
type Policy struct {
	Rules           map[string]string `json:"rules,omitempty"`
	Binaries        []string          `json:"binaries,omitempty"`
	Paths           []string          `json:"paths,omitempty"`
	ExcludeBinaries []string          `json:"exclude_binaries,omitempty"`
	ExcludePaths    []string          `json:"exclude_paths,omitempty"`
}

type RuleInfo struct {
	ID          string `json:"id"`
	Category    string `json:"category"`
	Description string `json:"description"`
	Mode        string `json:"mode"`
	Locked      bool   `json:"locked"`
}

var ErrInvalidPolicy = errors.New("guard: invalid policy")

func ParsePolicy(data []byte) (Policy, error) {
	var policy Policy
	if len(data) == 0 {
		return policy, nil
	}
	if err := json.Unmarshal(data, &policy); err != nil {
		return Policy{}, fmt.Errorf("%w: %v", ErrInvalidPolicy, err)
	}
	if err := policy.Validate(); err != nil {
		return Policy{}, err
	}
	return policy, nil
}

func (p Policy) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// ValidateGlobal checks a policy meant as the global one, which holds custom rules only.
func (p Policy) ValidateGlobal() error {
	if len(p.Rules) > 0 || len(p.ExcludeBinaries) > 0 || len(p.ExcludePaths) > 0 {
		return fmt.Errorf("%w: the global policy holds only blocked binaries and paths", ErrInvalidPolicy)
	}
	return p.Validate()
}

// Combine is what a project enforces: its own rule modes, the global binaries and paths it does not exclude, and its own binaries and paths.
func Combine(global, project Policy) Policy {
	return Policy{
		Rules:    project.Rules,
		Binaries: mergeExcluding(global.Binaries, project.ExcludeBinaries, project.Binaries, strings.EqualFold),
		Paths:    mergeExcluding(global.Paths, project.ExcludePaths, project.Paths, func(a, b string) bool { return a == b }),
	}
}

func mergeExcluding(global, exclude, own []string, same func(a, b string) bool) []string {
	var out []string
	add := func(value string) {
		for _, existing := range out {
			if same(existing, value) {
				return
			}
		}
		out = append(out, value)
	}
	for _, value := range global {
		excluded := false
		for _, ex := range exclude {
			if same(ex, value) {
				excluded = true
				break
			}
		}
		if !excluded {
			add(value)
		}
	}
	for _, value := range own {
		add(value)
	}
	return out
}

func (p Policy) Validate() error {
	if len(p.Rules) > maxPolicyRules || len(p.Binaries)+len(p.ExcludeBinaries) > maxPolicyBinaries || len(p.Paths)+len(p.ExcludePaths) > maxPolicyPaths {
		return fmt.Errorf("%w: too many entries", ErrInvalidPolicy)
	}
	known := knownRules()
	for id, value := range p.Rules {
		info, ok := known[id]
		if !ok {
			return fmt.Errorf("%w: unknown rule %q", ErrInvalidPolicy, id)
		}
		if info.Locked {
			return fmt.Errorf("%w: rule %q protects lemongrass and cannot be changed", ErrInvalidPolicy, id)
		}
		if value != overrideAllow && value != overrideDeny && value != overrideApproval {
			return fmt.Errorf("%w: rule %q has mode %q, want allow, deny or approval", ErrInvalidPolicy, id, value)
		}
	}
	for _, name := range p.ExcludeBinaries {
		if name == "" || strings.ContainsAny(name, " \t\n/\x00") {
			return fmt.Errorf("%w: %q is not a binary name", ErrInvalidPolicy, name)
		}
	}
	for _, pattern := range p.ExcludePaths {
		if pattern == "" || strings.ContainsRune(pattern, 0) {
			return fmt.Errorf("%w: %q is not a path", ErrInvalidPolicy, pattern)
		}
	}
	for _, name := range p.Binaries {
		if name == "" || strings.ContainsAny(name, " \t\n/\x00") {
			return fmt.Errorf("%w: %q is not a binary name", ErrInvalidPolicy, name)
		}
		if strings.EqualFold(name, "lgrass") {
			return fmt.Errorf("%w: lgrass cannot be blocked because the connectors depend on it", ErrInvalidPolicy)
		}
	}
	for _, pattern := range p.Paths {
		if pattern == "" || strings.ContainsRune(pattern, 0) {
			return fmt.Errorf("%w: %q is not a path", ErrInvalidPolicy, pattern)
		}
		if pattern != "~" && !strings.HasPrefix(pattern, "~/") && !filepath.IsAbs(pattern) {
			return fmt.Errorf("%w: path %q must be absolute or start with ~/", ErrInvalidPolicy, pattern)
		}
		if _, err := filepath.Match(pattern, ""); err != nil {
			return fmt.Errorf("%w: path pattern %q: %v", ErrInvalidPolicy, pattern, err)
		}
	}
	return nil
}

func (p Policy) blocksBinary(name string) bool {
	for _, blocked := range p.Binaries {
		if strings.EqualFold(blocked, name) {
			return true
		}
	}
	return false
}

func (p Policy) blocksPath(resolved, home string) bool {
	for _, pattern := range p.Paths {
		pattern = expandHomePattern(pattern, home)
		for current := resolved; ; {
			if ok, _ := filepath.Match(pattern, current); ok {
				return true
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
			current = parent
		}
	}
	return false
}

func expandHomePattern(pattern, home string) string {
	switch {
	case pattern == "~":
		return home
	case strings.HasPrefix(pattern, "~/"):
		return filepath.Join(home, pattern[2:])
	}
	return pattern
}

func modeName(mode Mode) string {
	if mode == ModeApproval {
		return overrideApproval
	}
	return overrideDeny
}

// Catalog lists every rule the user can see, catalog and engine rules alike, at its built-in mode.
func Catalog() []RuleInfo {
	var infos []RuleInfo
	for _, rule := range dangerCatalog("") {
		infos = append(infos, RuleInfo{ID: rule.ID, Category: categoryOf(rule.ID), Description: rule.Description, Mode: modeName(rule.Mode), Locked: lockedRules[rule.ID]})
	}
	return append(infos, engineRules...)
}

func knownRules() map[string]RuleInfo {
	known := map[string]RuleInfo{}
	for _, info := range Catalog() {
		known[info.ID] = info
	}
	return known
}
