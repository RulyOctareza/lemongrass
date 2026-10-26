// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package main

// Set by the Electron app on each agent tab it spawns, so the id is inherited by everything the tab runs.
const tabIDEnv = "LGRASS_TAB_ID"

// Read once in main(); empty for commands run outside a lemongrass tab. A model can override it on its own command line, so it routes and labels but never authorizes.
var tabID string

// actorFor is the caller-facing label a connector audit row is stamped with: the tab that made the call when known, the channel's own short id otherwise, never authorizing anything either way.
func actorFor(shortID string) string {
	if tabID != "" {
		return tabID
	}
	return shortID
}
