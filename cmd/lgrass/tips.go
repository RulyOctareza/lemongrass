// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"fmt"
	"os"
	"strconv"

	"github.com/faizalv/lemongrass/internal/session"
)

func cmdTips(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrass tips <add|list|remove> ...")
		os.Exit(1)
	}
	switch args[0] {
	case "add":
		cmdTipsAdd(args[1:])
	case "list":
		cmdTipsList(args[1:])
	case "remove":
		cmdTipsRemove(args[1:])
	default:
		fmt.Fprintf(os.Stderr, "unknown tips command: %s\n", args[0])
		os.Exit(1)
	}
}

func cmdTipsAdd(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, `usage: lgrass tips add "<message>"`)
		os.Exit(1)
	}
	proj := currentProject()
	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	id, err := store.AddTip(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("added tip %d.\n", id)
}

func cmdTipsList(args []string) {
	proj := currentProject()
	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	tips, err := store.ListTips()
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	if len(tips) == 0 {
		fmt.Println("lgrass: no custom tips configured for this project.")
		return
	}
	for _, t := range tips {
		fmt.Printf("%d  %s\n", t.ID, t.Message)
	}
}

func cmdTipsRemove(args []string) {
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "usage: lgrass tips remove <id>")
		os.Exit(1)
	}
	id, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: invalid id %q\n", args[0])
		os.Exit(1)
	}

	proj := currentProject()
	store, err := session.Open(session.DBPath(), proj.ID)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	defer store.Close()

	if err := store.DeleteTip(id); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
	fmt.Println("removed.")
}
