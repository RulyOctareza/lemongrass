// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestParseAsk(t *testing.T) {
	cases := []struct {
		name string
		yaml string
		err  string
	}{
		{"db", "type: db\ntables: [orders, customers]\nreason: monthly report\n", ""},
		{"rester", "type: rester\nmessage: |\n  I need rester X, GET on /orders\n", ""},
		{"command", "type: command\ncommand: |\n  git push origin \"$(git branch --show-current)\"\nreason: publish the fix\n", ""},
		{"empty", "", "empty"},
		{"no type", "reason: x\n", "needs a type"},
		{"unknown type", "type: shell\n", "unknown type"},
		{"unknown field", "type: db\ntables: [a]\nreason: r\nttl: 3600\n", "parsing"},
		{"db no tables", "type: db\nreason: r\n", "needs tables"},
		{"db no reason", "type: db\ntables: [a]\n", "needs a reason"},
		{"db with command", "type: db\ntables: [a]\nreason: r\ncommand: ls\n", "tables and reason only"},
		{"rester no message", "type: rester\nreason: r\n", "needs a message"},
		{"rester with tables", "type: rester\nmessage: m\ntables: [a]\n", "message and an optional reason"},
		{"command no reason", "type: command\ncommand: ls\n", "needs a reason"},
		{"command empty", "type: command\nreason: r\n", "exact command"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			req, err := parseAsk([]byte(tc.yaml))
			if tc.err == "" {
				if err != nil {
					t.Fatalf("parseAsk: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.err) {
				t.Fatalf("parseAsk error = %v, want %q (parsed %+v)", err, tc.err, req)
			}
		})
	}
	req, _ := parseAsk([]byte("type: command\ncommand: |\n  echo `date` $(id)\nreason: r\n"))
	if req.Command != "echo `date` $(id)" {
		t.Fatalf("command = %q, want it literal", req.Command)
	}
}

func TestFormatAsk(t *testing.T) {
	cases := []struct {
		ask  vault.Ask
		code int
		want string
	}{
		{vault.Ask{ID: "a1", Type: vault.AskDB, State: vault.AskAnswered, ShortID: "xy12"}, 0, "lgrass db xy12"},
		{vault.Ask{ID: "a1", Type: vault.AskRester, State: vault.AskAnswered, ShortID: "xy12"}, 0, "lgrass rester xy12 info"},
		{vault.Ask{ID: "a1", Type: vault.AskCommand, State: vault.AskAnswered, Result: &vault.AskResult{ExitCode: 1, OutputTail: "boom"}}, 0, "exit code 1.\nLast lines of its output:\nboom"},
		{vault.Ask{ID: "a1", Type: vault.AskCommand, State: vault.AskAnswered, Result: &vault.AskResult{ExitCode: -1, TimedOut: true}}, 0, "hit the timeout"},
		{vault.Ask{ID: "a1", Type: vault.AskCommand, State: vault.AskDeclined, Note: "not on main"}, 2, "declined by the user: not on main"},
		{vault.Ask{ID: "a1", Type: vault.AskCommand, State: vault.AskDeclined}, 2, "no note given"},
		{vault.Ask{ID: "a1", State: vault.AskExpired}, 2, "expired"},
		{vault.Ask{ID: "a1", Type: vault.AskCommand, State: vault.AskRunning}, 3, "lgrass ask show a1"},
		{vault.Ask{ID: "a1", State: vault.AskWaiting}, 3, "no answer yet"},
	}
	for _, tc := range cases {
		text, code := formatAsk(tc.ask)
		if code != tc.code || !strings.Contains(text, tc.want) {
			t.Errorf("formatAsk(%s %s) = %q, %d, want %q, %d", tc.ask.Type, tc.ask.State, text, code, tc.want, tc.code)
		}
	}
}
