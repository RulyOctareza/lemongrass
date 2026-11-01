// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/osdeps"
)

// cmdDeps prints the OS dependency check for the app. It exits 0 whatever the check found, since a missing tool is
// a result, not a failure; a cache that cannot be written is reported on stderr without withholding the report.
func cmdDeps(args []string) {
	fresh := false
	switch {
	case len(args) == 2 && args[0] == "check" && args[1] == "--json":
	case len(args) == 3 && args[0] == "check" && args[1] == "--json" && args[2] == "--fresh":
		fresh = true
	default:
		fmt.Fprintln(os.Stderr, "usage: lgrassd deps check --json [--fresh]")
		os.Exit(1)
	}
	report, err := osdeps.Check(context.Background(), osdeps.Real(), osdeps.CachePath(), fresh)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	if err := json.NewEncoder(os.Stdout).Encode(report); err != nil {
		fail(err)
	}
}
