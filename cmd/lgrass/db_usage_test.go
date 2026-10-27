// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"strings"
	"testing"

	"github.com/faizalv/lemongrass/internal/dbgate"
)

func TestDbUsageListsEveryAllowedFunction(t *testing.T) {
	text := dbUsageText()
	for _, engine := range []dbgate.Engine{dbgate.EngineMySQL, dbgate.EnginePostgres} {
		names := dbgate.FunctionNames(engine)
		if len(names) == 0 {
			t.Fatalf("%s has no allowlisted functions", engine)
		}
		for _, name := range names {
			if !strings.Contains(text, " "+name+",") && !strings.Contains(text, " "+name+"\n") {
				t.Errorf("usage text doesn't list %s function %s", engine, name)
			}
		}
	}
}

func TestWrapNamesKeepsWidth(t *testing.T) {
	out := wrapNames([]string{"alpha", "beta", "gamma", "delta"}, 14)
	for _, line := range strings.Split(strings.TrimSuffix(out, "\n"), "\n") {
		if len(line) > 14 {
			t.Errorf("line %q is wider than 14", line)
		}
	}
	if got := strings.Join(strings.Fields(strings.ReplaceAll(out, ",", "")), " "); got != "alpha beta gamma delta" {
		t.Errorf("wrapNames lost or reordered names: %q", got)
	}
}
