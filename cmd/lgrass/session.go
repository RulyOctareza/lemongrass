// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/internal/session"
)

func cmdSession(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrass session <list|tabs> ...")
		os.Exit(1)
	}
	switch args[0] {
	case "list":
		cmdSessionList(args[1:])
	case "tabs":
		cmdSessionTabs(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown session command: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdSessionList(args []string) {
	proj := currentProject()
	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	tabs, err := store.ListTabs()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	printed := 0
	for _, t := range tabs {
		if t.ID == tabID {
			continue
		}
		line := session.ShortID(t.ID) + "  " + t.Vendor
		if t.Title != "" {
			line += fmt.Sprintf("  %q", t.Title)
		}
		fmt.Println(line)
		printed++
	}
	if printed == 0 {
		fmt.Println("lgrass: no other tabs open in this project.")
	}
}
