// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/faizalv/lemongrass/internal/agent"
	"github.com/faizalv/lemongrass/internal/dbgate"
)

const dbUsageBase = `usage: lgrass db <short-id> --sql "<statement>" [--dry-run|--commit]

Runs a statement against the database a channel grants access to, checked against what the
statement actually references -- the channel's own scope decides what's allowed, nothing declared
on the command line is trusted on its own. A read (SELECT/SHOW/DESCRIBE/EXPLAIN) runs immediately.
A write (INSERT/UPDATE/DELETE) requires exactly one of --dry-run (runs it in a transaction and
rolls back, reporting the row count it would have affected) or --commit (runs it and actually
commits). An UPDATE/DELETE with no WHERE clause is rejected outright, regardless of scope.

A channel granted the performance operation can also SELECT from the performance views below,
referenced directly in the statement. Quote a MySQL view name containing $ in backticks, like
sys.x$statement_analysis; a Postgres view is referenced by its bare name, or as pg_catalog.name
when the statement qualifies it. Columns holding raw statement text are never readable: name the
columns instead of using *, where a listed column is barred.
`

func dbUsageText() string {
	var b strings.Builder
	b.WriteString(dbUsageBase)
	for _, e := range []struct {
		label  string
		engine dbgate.Engine
	}{{"MySQL", dbgate.EngineMySQL}, {"Postgres", dbgate.EnginePostgres}} {
		fmt.Fprintf(&b, "\n%s performance views:\n  %s\n", e.label, strings.Join(dbgate.PerfViewNames(e.engine), "\n  "))
		restricted := dbgate.PerfViewRestrictedColumns(e.engine)
		names := make([]string, 0, len(restricted))
		for name := range restricted {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			fmt.Fprintf(&b, "  barred on %s: %s\n", name, strings.Join(restricted[name], ", "))
		}
	}
	return b.String()
}

func cmdDb(args []string) {
	if len(args) == 0 {
		fmt.Print(dbUsageText())
		return
	}

	shortID := args[0]
	var sqlText string
	var dryRun, commit bool
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--sql":
			i++
			if i < len(args) {
				sqlText = args[i]
			}
		case "--dry-run":
			dryRun = true
		case "--commit":
			commit = true
		}
	}
	if sqlText == "" {
		fmt.Fprint(os.Stderr, dbUsageText())
		os.Exit(1)
	}
	if dryRun && commit {
		fmt.Fprintln(os.Stderr, "error: pass exactly one of --dry-run or --commit, not both")
		os.Exit(1)
	}

	client := &agent.Client{SocketPath: agent.SocketPath()}

	var out []byte
	var err error
	switch {
	case dryRun || commit:
		var result dbgate.WriteResult
		result, err = client.Execute(shortID, sqlText, commit, actorFor(shortID))
		if err == nil {
			out, err = json.Marshal(result)
		}
	default:
		var result dbgate.QueryResult
		result, err = client.Query(shortID, sqlText)
		if err == nil {
			out, err = json.Marshal(result)
		}
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(out))
}
