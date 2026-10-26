// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

import (
	"fmt"
	"os"

	"github.com/faizalv/lemongrass/cmd/lgrass/version"
	"github.com/faizalv/lemongrass/internal/hook"
)

// Set by the Electron app on each agent tab it spawns, so the id is inherited by everything the tab runs.
const tabIDEnv = "LGRASS_TAB_ID"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}
	switch os.Args[1] {
	case "version":
		fmt.Println(version.Version)
	case "--help", "-h", "help":
		usage()
	case "vault":
		cmdVault(os.Args[2:])
	case "agent":
		cmdAgent(os.Args[2:])
	case "node":
		cmdNode(os.Args[2:])
	case "tabs":
		cmdTabs(os.Args[2:])
	case "accounts":
		cmdAccounts(os.Args[2:])
	case "trust":
		cmdTrust(os.Args[2:])
	case "nudge":
		cmdNudge(os.Args[2:])
	case "hook":
		hook.Run(os.Args[2:], os.Getenv(tabIDEnv))
	default:
		fmt.Fprintf(os.Stderr, "unknown command: %s\n", os.Args[1])
		usage()
		os.Exit(1)
	}
}

func usage() {
	fmt.Print(`lgrassd -- lemongrass's app-invoked binary

COMMANDS
  vault run                         Starts the credential-vault daemon, listening on a unix socket
                                     under ~/.lemongrass
  agent run                         Starts the gatekeeper agent daemon, listening on a unix socket
                                     under ~/.lemongrass; maps short channel ids to real vault
                                     channels and forwards queries to a running vault daemon; also
                                     keeps the private history of every registered project's biblio
  node run                          Starts the node daemon, which serves granted books and tasks
                                     read-only over a tunnel while node.json says it is open
  tabs list                         Tab id to session id map for the project at the working directory, as JSON
  tabs register <tab-id> <vendor>   Records the agent vendor a tab runs
  tabs title <tab-id> <title>       Records the shell title a tab currently shows
  tabs clear                        Drops every registered tab of the project, keeping their saved sessions
  tabs forget <tab-id>              Drops a closed tab's records
  accounts list                     The Claude accounts beside the primary, as JSON
  accounts add <name>               Creates ~/.claude-<name>, links the shared folders and registers the hooks
  accounts remove <name>            Deletes ~/.claude-<name> and forgets the account
  trust <path> [--target <t>]       Marks a project trusted for Claude and Codex; t is primary, codex or an account name
  nudge <tab-id>                    Prints the tab's pending thread nudge and marks it sent
  hook <event>                      Invoked by Claude Code's and Codex's own hook systems, reads hook JSON off stdin

  version                           Print version
`)
}
