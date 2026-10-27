// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package session

import (
	"strings"
	"testing"
)

func TestResolveSignableBuiltins(t *testing.T) {
	for _, id := range []string{MemoryWriteWord} {
		got, ok, err := ResolveSignable(t.TempDir(), id)
		if err != nil || !ok {
			t.Fatalf("ResolveSignable(%q) = ok %v, err %v, want a registered word", id, ok, err)
		}
		if got.Pledge == "" {
			t.Errorf("ResolveSignable(%q) has no pledge", id)
		}
	}
}

func TestResolveSignableRejectsUnknownWords(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"--help", "memory-feedback-law", "", "I-WRITE-MEMORY"} {
		if _, ok, err := ResolveSignable(dir, id); ok || err != nil {
			t.Errorf("ResolveSignable(%q) = ok %v, err %v, want unregistered", id, ok, err)
		}
	}
}

func TestResolveSignableUsesEveryRegisteredSource(t *testing.T) {
	saved := signSources
	defer func() { signSources = saved }()
	signSources = append(append([]SignSource{}, saved...), func(string) ([]Signable, error) {
		return []Signable{{ID: "user-set", Pledge: "mine"}}, nil
	})
	got, ok, err := ResolveSignable(t.TempDir(), "user-set")
	if err != nil || !ok || got.Pledge != "mine" {
		t.Errorf("added source = %+v, ok %v, err %v", got, ok, err)
	}
}

func TestDeniesStayShort(t *testing.T) {
	for name, msg := range map[string]string{
		"memory": FormatMemoryFeedbackDeny(),
	} {
		if len(strings.Fields(msg)) > 60 {
			t.Errorf("%s deny has %d words, want at most 60", name, len(strings.Fields(msg)))
		}
	}
}
