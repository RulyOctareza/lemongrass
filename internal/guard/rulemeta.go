// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// Catalog rules that protect lemongrass itself and cannot be changed by a policy.
var lockedRules = map[string]bool{
	"keyring": true,
}

// Verdicts the engine produces outside the catalog. The locked ones cannot be changed by a policy.
var engineRules = []RuleInfo{
	{ID: "secret-path", Category: "Lemongrass", Description: "access to credentials or lemongrass private state", Mode: overrideDeny, Locked: true},
	{ID: "guard-config", Category: "Lemongrass", Description: "changing the agent settings, hooks or the lgrass binaries", Mode: overrideDeny, Locked: true},
	{ID: "dynamic-command", Category: "Lemongrass", Description: "the command name is computed at runtime, so it cannot be checked", Mode: overrideDeny, Locked: true},
	{ID: "unparseable-command", Category: "Lemongrass", Description: "the shell command could not be parsed, so it cannot be checked", Mode: overrideDeny, Locked: true},
	{ID: "download-exec", Category: "Network", Description: "running downloaded code", Mode: overrideDeny},
	{ID: "raw-device-write", Category: "System", Description: "writing to a raw disk device", Mode: overrideDeny},
}

var lockedEngineIDs = map[string]bool{
	"secret-path": true, "guard-config": true, "dynamic-command": true, "unparseable-command": true,
}

func isLocked(id string) bool {
	return lockedRules[id] || lockedEngineIDs[id]
}

func categoryOf(id string) string {
	switch {
	case strings.HasPrefix(id, "git-"):
		return "Git"
	}
	switch id {
	case "rm-force", "rm-plain", "find-delete", "shred", "unlink":
		return "Deletion"
	case "disk-tool", "mkfs", "privilege", "power", "firewall", "kill-by-name", "kill-all", "systemctl-stop", "recursive-perms", "crontab", "at-jobs":
		return "System"
	case "curl-upload", "wget-upload", "raw-network", "remote-shell", "rsync-remote":
		return "Network"
	case "keyring":
		return "Lemongrass"
	case "db-client":
		return "Database"
	case "docker-destroy", "kubectl-delete", "terraform-destroy", "tofu-destroy", "cloud-delete",
		"package-publish", "package-publish-yarn", "package-publish-pnpm", "cargo-publish", "twine":
		return "Infrastructure"
	}
	return "Other"
}
