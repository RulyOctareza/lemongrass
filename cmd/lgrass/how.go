// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/faizalv/lemongrass/internal/bibliohistory"
	"github.com/faizalv/lemongrass/internal/how"
	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/session"
)

func cmdHow(args []string) {
	out, err := how.RenderEnv(howEnv(currentProject()), args)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Print(out)
}

// The sources open lazily, so an action that never asks pays nothing, and a source that fails to open reads as
// unavailable rather than failing the command.
func howEnv(proj project.Project) how.Env {
	repo := bibliohistory.New(bibliohistory.GitDir(proj.ID), filepath.Join(proj.Path, "biblio"))
	return how.Env{
		Root: proj.Path,
		TaskStarted: func(slug string) (time.Time, bool) {
			t, ok, err := repo.FirstTime("scratchpad/" + slug)
			return t, ok && err == nil
		},
		EditsSince: func(since time.Time) []how.Edit {
			store, err := session.Open(session.DBPath(), proj.ID)
			if err != nil {
				return nil
			}
			defer store.Close()
			touches, err := store.FileActivitySince(since)
			if err != nil {
				return nil
			}
			edits := make([]how.Edit, len(touches))
			for i, t := range touches {
				edits[i] = how.Edit{Session: t.SessionID, Path: t.FilePath}
			}
			return edits
		},
	}
}
