// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/bibliohistory"
	"github.com/faizalv/lemongrass/internal/project"
	"github.com/faizalv/lemongrass/internal/session"
)

func cmdProject(args []string) {
	if len(args) != 2 || args[0] != "purge" {
		fmt.Fprintln(os.Stderr, "usage: lgrassd project purge <project-id>")
		os.Exit(1)
	}
	if err := purgeProject(args[1]); err != nil {
		fail(err)
	}
}

// purgeProject removes the Go-owned data of a deleted project: its session store rows and its biblio history.
// The app removes its own files and the registry entry after this succeeds, so a failure here is retried on its next
// purge run. A project that is not marked deleted is refused, so a stray call never wipes a live project.
func purgeProject(id string) error {
	projects, err := project.ListAll()
	if err != nil {
		return err
	}
	var found *project.Project
	for i := range projects {
		if projects[i].ID == id {
			found = &projects[i]
		}
	}
	if found == nil {
		return fmt.Errorf("project %q is not registered", id)
	}
	if found.DeletedAt == "" {
		return errors.New("project is not deleted, refusing to purge it")
	}

	store, err := session.Open(session.DBPath(), id)
	if err != nil {
		return err
	}
	defer store.Close()
	if err := store.PurgeProject(); err != nil {
		return err
	}
	return os.RemoveAll(bibliohistory.GitDir(id))
}
