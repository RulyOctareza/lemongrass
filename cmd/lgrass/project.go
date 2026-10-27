// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/project"
)

// Shared by every subcommand that needs the registered project for cwd. Exits
// the process on failure rather than returning an error, since every caller
// would just do that anyway.
func currentProject() project.Project {
	p, err := project.ResolveCwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	return p
}
