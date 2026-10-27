// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package bibliothek

import (
	"bytes"
	"testing"
)

func TestPromptCutsTheAutoloadTrigger(t *testing.T) {
	text, err := Prompt()
	if err != nil {
		t.Fatal(err)
	}
	for _, gone := range []string{"name: bibliothek", "MANDATORY: invoke bibliothek", "Two things get checked every session"} {
		if bytes.Contains(text, []byte(gone)) {
			t.Errorf("prompt still carries %q", gone)
		}
	}
	for _, kept := range []string{"## Tier 2", "**The laws summary.**"} {
		if !bytes.Contains(text, []byte(kept)) {
			t.Errorf("prompt lost %q", kept)
		}
	}
}

func TestPromptFailsWhenTheMarkersMove(t *testing.T) {
	saved := Skill
	defer func() { Skill = saved }()
	Skill = []byte("---\nname: bibliothek\n---\n\n## Bootstrapping\n\nreworded\n")
	if _, err := Prompt(); err == nil {
		t.Fatal("expected an error when the cut markers are missing")
	}
}
