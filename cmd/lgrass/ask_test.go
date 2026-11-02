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
		{"db", "type: db\ntables: [orders, customers]\noperations: [select, SHOW]\nreason: monthly report\n", ""},
		{"db no operations", "type: db\ntables: [a]\nreason: r\n", "operations"},
		{"db unknown operation", "type: db\ntables: [a]\noperations: [drop]\nreason: r\n", "not a known operation"},
		{"rester", "type: rester\nmessage: |\n  I need rester X, GET on /orders\nmethods: [get, POST]\n", ""},
		{"rester no methods", "type: rester\nmessage: m\n", "methods"},
		{"command", "type: command\ncommand: |\n  git push origin \"$(git branch --show-current)\"\nreason: publish the fix\n", ""},
		{"empty", "", "empty"},
		{"no type", "reason: x\n", "needs a type"},
		{"unknown type", "type: shell\n", "unknown type"},
		{"unknown field", "type: db\ntables: [a]\noperations: [select]\nreason: r\nttl: 3600\n", "parsing"},
		{"db no tables", "type: db\nreason: r\n", "needs tables"},
		{"db no reason", "type: db\ntables: [a]\noperations: [select]\n", "needs a reason"},
		{"db with command", "type: db\ntables: [a]\noperations: [select]\nreason: r\ncommand: ls\n", "tables, operations and reason only"},
		{"rester no message", "type: rester\nreason: r\nmethods: [GET]\n", "needs a message"},
		{"rester with tables", "type: rester\nmessage: m\ntables: [a]\n", "message, methods and an optional reason"},
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
	db, _ := parseAsk([]byte("type: db\ntables: [a]\noperations: [SELECT]\nreason: r\n"))
	if strings.Join(db.Operations, ",") != "select" {
		t.Fatalf("operations = %v, want lower case", db.Operations)
	}
	rester, _ := parseAsk([]byte("type: rester\nmessage: m\nmethods: [get]\n"))
	if strings.Join(rester.Methods, ",") != "GET" {
		t.Fatalf("methods = %v, want upper case", rester.Methods)
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
		{vault.Ask{ID: "a1", Type: vault.AskDB, State: vault.AskAnswered, ShortID: "xy12", Granted: &vault.AskGrant{Tables: []string{"orders"}, Operations: []string{"select"}}, RemovedTables: []vault.RemovedTable{{Table: "refunds", Reason: "does not exist in connection shop"}}}, 0, "Granted tables: orders. Granted operations: select.\nRemoved table refunds: does not exist in connection shop."},
		{vault.Ask{ID: "a1", Type: vault.AskDB, State: vault.AskDeclined, Note: "no", RemovedTables: []vault.RemovedTable{{Table: "refunds", Reason: "removed by the user"}}}, 2, "declined by the user: no\nRemoved table refunds: removed by the user."},
		{vault.Ask{ID: "a1", Type: vault.AskRester, State: vault.AskAnswered, ShortID: "xy12", Granted: &vault.AskGrant{Methods: []string{"GET"}}}, 0, "Granted methods: GET."},
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
