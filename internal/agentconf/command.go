// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package agentconf

// HookCommand is the command line that runs lgrassdPath with args, in the form the agents' hook runners read on this
// OS. Every hook and statusline command lemongrass registers is built here, so quoting changes in one place.
func HookCommand(lgrassdPath string, args ...string) string {
	command := quoteCommandPath(lgrassdPath)
	for _, arg := range args {
		command += " " + arg
	}
	return command
}
