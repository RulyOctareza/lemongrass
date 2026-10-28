// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package bibliothek

import (
	"bytes"
	_ "embed"
	"errors"
)

// Skill is bibliothek's SKILL.md at the commit the submodule pins.
//
//go:embed repo/skill/SKILL.md
var Skill []byte

var (
	triggerStart = []byte("Two things get checked every session")
	triggerEnd   = []byte("**The laws summary.**")
)

// Prompt returns the skill text for a system prompt that already carries it, so the autoload trigger instructions are cut.
func Prompt() ([]byte, error) {
	body, err := stripFrontmatter(Skill)
	if err != nil {
		return nil, err
	}
	start := bytes.Index(body, triggerStart)
	end := bytes.Index(body, triggerEnd)
	if start < 0 || end < start {
		return nil, errors.New("bibliothek: autoload trigger paragraphs not found in SKILL.md")
	}
	out := append([]byte{}, body[:start]...)
	return append(out, body[end:]...), nil
}

func stripFrontmatter(text []byte) ([]byte, error) {
	fence := []byte("---\n")
	if !bytes.HasPrefix(text, fence) {
		return nil, errors.New("bibliothek: SKILL.md has no frontmatter")
	}
	end := bytes.Index(text[len(fence):], []byte("\n---\n"))
	if end < 0 {
		return nil, errors.New("bibliothek: SKILL.md frontmatter is not closed")
	}
	return bytes.TrimLeft(text[len(fence)+end+len("\n---\n"):], "\n"), nil
}
