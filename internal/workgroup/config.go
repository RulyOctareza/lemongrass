// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// Package workgroup parses workgroup configs and talks to the lemongrass app that spawns a group's tabs.
package workgroup

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/faizalv/lemongrass/internal/session"

	"gopkg.in/yaml.v3"
)

const (
	MaxSkills = 8

	// Every thinker loads this skill, so a config never has to list it.
	ThinkerSkill      = session.ThinkerSkill
	MaxPromptRunes    = 3000
	MaxNameRunes      = 120
	defaultLeaderName = "leader"
)

var (
	labelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,29}$`)
	modelPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,79}$`)
	skillPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)
	vendors      = map[string]bool{"claude": true, "codex": true}
)

type Thinker struct {
	Label      string   `yaml:"label"`
	Vendor     string   `yaml:"vendor"`
	Model      string   `yaml:"model"`
	Prompt     string   `yaml:"prompt"`
	PromptFile string   `yaml:"prompt_file"`
	Skills     []string `yaml:"skills"`
}

type Config struct {
	Name        string    `yaml:"name"`
	LeaderLabel string    `yaml:"leader_label"`
	Thinkers    []Thinker `yaml:"thinkers"`
}

// JSON is accepted because it is valid YAML. Unknown keys are an error, and each prompt_file is read relative to the config's directory into Prompt.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("workgroup: reading config: %w", err)
	}
	return Parse(data, filepath.Dir(path))
}

func Parse(data []byte, baseDir string) (Config, error) {
	cfg, err := decode(data)
	if err != nil {
		return Config{}, err
	}

	cfg.Name = strings.TrimSpace(cfg.Name)
	if cfg.Name == "" {
		return Config{}, errors.New("workgroup: the config needs a name that identifies this group, and it must differ from every other workgroup's name")
	}
	if utf8.RuneCountInString(cfg.Name) > MaxNameRunes {
		return Config{}, fmt.Errorf("workgroup: name is longer than %d characters", MaxNameRunes)
	}
	if cfg.LeaderLabel == "" {
		cfg.LeaderLabel = defaultLeaderName
	}
	if !labelPattern.MatchString(cfg.LeaderLabel) {
		return Config{}, fmt.Errorf("workgroup: leader_label %q must be 1 to 30 letters, digits, dashes or underscores", cfg.LeaderLabel)
	}
	if err := validateThinkers(cfg.Thinkers, []string{cfg.LeaderLabel}, baseDir); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// LoadAddition reads the config of `lgrass workgroup add`, which lists only thinkers, since the group's name and leader come from the live group. A new label must not repeat any of taken, the group's current labels.
func LoadAddition(path string, taken []string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("workgroup: reading config: %w", err)
	}
	return ParseAddition(data, filepath.Dir(path), taken)
}

func ParseAddition(data []byte, baseDir string, taken []string) (Config, error) {
	cfg, err := decode(data)
	if err != nil {
		return Config{}, err
	}
	if cfg.Name != "" || cfg.LeaderLabel != "" {
		return Config{}, errors.New("workgroup: an add config lists only thinkers; the group's name and leader come from the live group")
	}
	if err := validateThinkers(cfg.Thinkers, taken, baseDir); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func decode(data []byte) (Config, error) {
	var cfg Config
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("workgroup: parsing config: %w", err)
	}
	return cfg, nil
}

// Checks each thinker in place and reads its prompt_file. A label may not repeat another thinker's or any of taken, compared ignoring case.
func validateThinkers(thinkers []Thinker, taken []string, baseDir string) error {
	if len(thinkers) == 0 {
		return errors.New("workgroup: the config lists no thinkers")
	}
	takenSet := map[string]bool{}
	for _, label := range taken {
		takenSet[strings.ToLower(label)] = true
	}
	seen := map[string]bool{}
	for i := range thinkers {
		c := &thinkers[i]
		if !labelPattern.MatchString(c.Label) {
			return fmt.Errorf("workgroup: thinker %d label %q must be 1 to 30 letters, digits, dashes or underscores", i+1, c.Label)
		}
		if takenSet[strings.ToLower(c.Label)] {
			return fmt.Errorf("workgroup: label %q is already taken in the group", c.Label)
		}
		if seen[strings.ToLower(c.Label)] {
			return fmt.Errorf("workgroup: label %q is used more than once", c.Label)
		}
		seen[strings.ToLower(c.Label)] = true
		if !vendors[c.Vendor] {
			return fmt.Errorf("workgroup: thinker %q vendor %q must be claude or codex", c.Label, c.Vendor)
		}
		if c.Model != "" && !modelPattern.MatchString(c.Model) {
			return fmt.Errorf("workgroup: thinker %q model %q is not a valid model name", c.Label, c.Model)
		}
		if len(c.Skills) > MaxSkills {
			return fmt.Errorf("workgroup: thinker %q lists %d skills and the limit is %d", c.Label, len(c.Skills), MaxSkills)
		}
		var skills []string
		for _, name := range c.Skills {
			if !skillPattern.MatchString(name) {
				return fmt.Errorf("workgroup: thinker %q skill %q is not a valid skill name", c.Label, name)
			}
			if name != ThinkerSkill && !containsString(skills, name) {
				skills = append(skills, name)
			}
		}
		c.Skills = skills
		if (c.Prompt == "") == (c.PromptFile == "") {
			return fmt.Errorf("workgroup: thinker %q needs exactly one of prompt and prompt_file", c.Label)
		}
		if c.PromptFile != "" {
			file := c.PromptFile
			if !filepath.IsAbs(file) {
				file = filepath.Join(baseDir, file)
			}
			text, err := os.ReadFile(file)
			if err != nil {
				return fmt.Errorf("workgroup: thinker %q prompt_file: %w", c.Label, err)
			}
			c.Prompt, c.PromptFile = string(text), ""
		}
		c.Prompt = strings.TrimSpace(c.Prompt)
		if c.Prompt == "" {
			return fmt.Errorf("workgroup: thinker %q prompt is empty", c.Label)
		}
		if n := utf8.RuneCountInString(c.Prompt); n > MaxPromptRunes {
			return fmt.Errorf("workgroup: thinker %q prompt is %d characters and the limit is %d", c.Label, n, MaxPromptRunes)
		}
	}
	return nil
}

func containsString(list []string, v string) bool {
	for _, item := range list {
		if item == v {
			return true
		}
	}
	return false
}

// Every skill a thinker must load: the thinker skill first, then the ones the leader listed.
func (c Thinker) RequiredSkills() []string {
	out := []string{ThinkerSkill}
	for _, name := range c.Skills {
		if !session.CarriedInPrompt(c.Vendor, name) {
			out = append(out, name)
		}
	}
	return out
}

var skillDirs = map[string]string{"claude": ".claude", "codex": ".codex"}

// Checks that each thinker's required skills are installed for its vendor, so a group is never approved with a skill nobody can load.
func (cfg Config) CheckSkillsInstalled(home string) error {
	for _, c := range cfg.Thinkers {
		root := filepath.Join(home, skillDirs[c.Vendor], "skills")
		for _, name := range c.RequiredSkills() {
			if _, err := os.Stat(filepath.Join(root, name, "SKILL.md")); err != nil {
				return fmt.Errorf("workgroup: thinker %q needs the skill %q, which is not installed for %s at %s. Installed: %s", c.Label, name, c.Vendor, root, installedSkills(root))
			}
		}
	}
	return nil
}

func installedSkills(root string) string {
	entries, err := os.ReadDir(root)
	if err != nil {
		return "none"
	}
	var names []string
	for _, e := range entries {
		if _, err := os.Stat(filepath.Join(root, e.Name(), "SKILL.md")); err == nil {
			names = append(names, e.Name())
		}
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
