// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
)

func TestSignRequest(t *testing.T) {
	t.Setenv("CLAUDE_CODE_SESSION_ID", "claude-session")
	cases := []struct {
		name        string
		args        []string
		wantID      string
		wantSession string
		wantErr     bool
	}{
		{"Claude environment", []string{"review"}, "review", "claude-session", false},
		{"explicit Codex session", []string{"--session-id", "codex-session", "review"}, "review", "codex-session", false},
		{"missing word", nil, "", "", true},
		{"missing session flag value", []string{"--session-id"}, "", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			id, sessionID, err := signRequest(c.args, "word")
			if (err != nil) != c.wantErr {
				t.Fatalf("signRequest(%v) error = %v, wantErr %v", c.args, err, c.wantErr)
			}
			if id != c.wantID || sessionID != c.wantSession {
				t.Errorf("signRequest(%v) = (%q, %q), want (%q, %q)", c.args, id, sessionID, c.wantID, c.wantSession)
			}
		})
	}
}

func TestSignAcknowledgement(t *testing.T) {
	cases := []struct {
		name string
		in   session.Signable
		want string
	}{
		{"with pledge", session.Signable{ID: "review", Pledge: "I read it."}, "signed review. I read it."},
		{"without pledge", session.Signable{ID: "review"}, "signed review."},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := signAcknowledgement(c.in); got != c.want {
				t.Errorf("signAcknowledgement(%v) = %q, want %q", c.in, got, c.want)
			}
		})
	}
}

func TestSignHelpRequested(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want bool
	}{
		{"no arguments", nil, true},
		{"short flag", []string{"-h"}, true},
		{"long flag", []string{"--help"}, true},
		{"flag after session id", []string{"--session-id", "s", "--help"}, true},
		{"a word", []string{"review"}, false},
		{"session id and word", []string{"--session-id", "s", "review"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := signHelpRequested(c.args); got != c.want {
				t.Errorf("signHelpRequested(%v) = %v, want %v", c.args, got, c.want)
			}
		})
	}
}

func TestSignUsageNamesNoRegisteredWord(t *testing.T) {
	for _, id := range []string{session.MemoryWriteWord} {
		if strings.Contains(signUsage, id) {
			t.Errorf("signUsage names the registered word %q", id)
		}
	}
}
