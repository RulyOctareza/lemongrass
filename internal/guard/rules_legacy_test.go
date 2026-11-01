// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// The closures below are the catalog as it was before the engine, kept only as an oracle: every call one of them matched must still match the same rule id.

type legacyRule struct {
	ID          string
	Description string
	Mode        Mode
	Match       func(call shellCall) bool
}

func legacyGitRule(id, description string, mode Mode, match func(sub string, rest []string) bool) legacyRule {
	return legacyRule{ID: id, Description: description, Mode: mode, Match: func(call shellCall) bool {
		if call.Name != "git" {
			return false
		}
		sub, rest := gitSubcommand(call.Args)
		return sub != "" && match(sub, rest)
	}}
}

func legacyNameRule(id, description string, names ...string) legacyRule {
	return legacyRule{ID: id, Description: description, Mode: ModeDeny, Match: func(call shellCall) bool {
		for _, name := range names {
			if call.Name == name {
				return true
			}
		}
		return false
	}}
}

func legacyPrefixRule(id, description, prefix string) legacyRule {
	return legacyRule{ID: id, Description: description, Mode: ModeDeny, Match: func(call shellCall) bool {
		return strings.HasPrefix(call.Name, prefix)
	}}
}

func legacyToolSubRule(id, description, tool string, subs ...string) legacyRule {
	return legacyRule{ID: id, Description: description, Mode: ModeDeny, Match: func(call shellCall) bool {
		if call.Name != tool {
			return false
		}
		for _, arg := range call.Args {
			if strings.HasPrefix(arg, "-") {
				continue
			}
			for _, sub := range subs {
				if arg == sub {
					return true
				}
			}
			return false
		}
		return false
	}}
}

func legacyCatalog(home string) []legacyRule {
	return []legacyRule{
		{ID: "rm-force", Description: "rm with a force or recursive flag", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "rm" && (hasShortFlag(call.Args, "frR") || hasLongFlag(call.Args, "--force", "--recursive", "--no-preserve-root"))
		}},
		{ID: "rm-plain", Description: "removing a file", Mode: ModeAsk, Match: func(call shellCall) bool {
			return call.Name == "rm"
		}},
		{ID: "find-delete", Description: "find -delete", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "find" && containsArg(call.Args, "-delete")
		}},
		legacyNameRule("shred", "overwriting files unrecoverably", "shred", "srm", "wipe"),
		legacyNameRule("unlink", "unlinking a file", "unlink"),

		legacyGitRule("git-push-force", "a forced or deleting git push", ModeDeny, func(sub string, rest []string) bool {
			if sub != "push" {
				return false
			}
			if hasShortFlag(rest, "fd") || hasLongFlag(rest, "--force", "--force-with-lease", "--force-if-includes", "--mirror", "--delete", "--prune") {
				return true
			}
			for _, arg := range rest {
				if strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, ":") {
					return true
				}
			}
			return false
		}),
		legacyGitRule("git-reset-hard", "git reset --hard", ModeDeny, func(sub string, rest []string) bool {
			return sub == "reset" && containsArg(rest, "--hard", "--merge", "--keep")
		}),
		legacyGitRule("git-clean", "git clean with force", ModeDeny, func(sub string, rest []string) bool {
			return sub == "clean" && (hasShortFlag(rest, "f") || hasLongFlag(rest, "--force"))
		}),
		legacyGitRule("git-branch-delete", "git branch force delete", ModeDeny, func(sub string, rest []string) bool {
			return sub == "branch" && (hasShortFlag(rest, "D") || (containsArg(rest, "--delete") && hasLongFlag(rest, "--force")))
		}),
		legacyGitRule("git-stash-drop", "dropping or clearing stashes", ModeDeny, func(sub string, rest []string) bool {
			return sub == "stash" && len(rest) > 0 && (rest[0] == "drop" || rest[0] == "clear")
		}),
		legacyGitRule("git-history-rewrite", "rewriting git history", ModeDeny, func(sub string, rest []string) bool {
			return sub == "filter-branch" || sub == "filter-repo"
		}),
		legacyGitRule("git-reflog-expire", "expiring or deleting the reflog", ModeDeny, func(sub string, rest []string) bool {
			return sub == "reflog" && len(rest) > 0 && (rest[0] == "expire" || rest[0] == "delete")
		}),
		legacyGitRule("git-prune", "pruning unreachable git objects", ModeDeny, func(sub string, rest []string) bool {
			return sub == "prune" || (sub == "gc" && hasLongFlag(rest, "--prune"))
		}),
		legacyGitRule("git-add", "staging changes", ModeAsk, func(sub string, rest []string) bool {
			return sub == "add"
		}),
		legacyGitRule("git-commit", "committing", ModeAsk, func(sub string, rest []string) bool {
			return sub == "commit"
		}),

		legacyNameRule("disk-tool", "a raw disk or filesystem tool", "dd", "fdisk", "sfdisk", "cfdisk", "parted", "wipefs", "sgdisk", "mkswap", "cryptsetup", "losetup"),
		legacyPrefixRule("mkfs", "creating a filesystem", "mkfs"),
		legacyNameRule("privilege", "privilege escalation", "sudo", "su", "doas", "pkexec", "run0"),
		legacyNameRule("power", "shutting down or rebooting the machine", "shutdown", "reboot", "poweroff", "halt", "telinit"),
		legacyNameRule("firewall", "changing the firewall", "iptables", "ip6tables", "nft", "ufw", "firewall-cmd"),
		legacyNameRule("kill-by-name", "killing processes by name", "pkill", "killall"),
		{ID: "kill-all", Description: "killing every process", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "kill" && containsArg(call.Args, "-1")
		}},
		legacyToolSubRule("systemctl-stop", "stopping, disabling or masking a service", "systemctl", "stop", "disable", "mask", "kill", "poweroff", "reboot", "halt", "suspend", "hibernate", "isolate"),
		{ID: "recursive-perms", Description: "recursive chmod or chown on a broad path", Mode: ModeDeny, Match: func(call shellCall) bool {
			if call.Name != "chmod" && call.Name != "chown" && call.Name != "chgrp" {
				return false
			}
			return (hasShortFlag(call.Args, "R") || hasLongFlag(call.Args, "--recursive")) && broadRecursiveTarget(call.Args, home)
		}},
		{ID: "crontab", Description: "changing the crontab", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "crontab" && !(len(call.Args) == 1 && call.Args[0] == "-l")
		}},
		legacyNameRule("at-jobs", "scheduling jobs with at", "at", "batch"),

		{ID: "curl-upload", Description: "curl sending a local file", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "curl" && curlSendsFile(call.Args)
		}},
		{ID: "wget-upload", Description: "wget sending a local file", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "wget" && hasLongFlag(call.Args, "--post-file", "--body-file")
		}},
		legacyNameRule("raw-network", "opening a raw network connection", "nc", "ncat", "netcat", "socat", "telnet"),
		legacyNameRule("remote-shell", "a remote shell or file copy", "ssh", "scp", "sftp", "sshfs", "mosh"),
		{ID: "rsync-remote", Description: "rsync to or from a remote host", Mode: ModeDeny, Match: func(call shellCall) bool {
			if call.Name != "rsync" {
				return false
			}
			if hasShortFlag(call.Args, "e") || hasLongFlag(call.Args, "--rsh") {
				return true
			}
			for _, arg := range call.Args {
				if remoteRsyncArg.MatchString(arg) {
					return true
				}
			}
			return false
		}},

		legacyNameRule("keyring", "reading the OS keyring", "secret-tool", "keyctl"),
		legacyNameRule("db-client", "a direct database client, use lgrass db instead", "mysql", "mariadb", "psql", "mongosh", "mongo", "redis-cli", "sqlcmd", "pg_dump", "pg_dumpall", "pg_restore", "mysqldump", "mysqladmin", "mariadb-dump", "cockroach", "clickhouse-client"),

		{ID: "docker-destroy", Description: "removing docker containers, images, volumes or networks", Mode: ModeDeny, Match: func(call shellCall) bool {
			if call.Name != "docker" && call.Name != "podman" {
				return false
			}
			var subs []string
			for _, arg := range call.Args {
				if !strings.HasPrefix(arg, "-") && len(subs) < 2 {
					subs = append(subs, arg)
				}
			}
			if len(subs) == 0 {
				return false
			}
			if subs[0] == "rm" || subs[0] == "rmi" || subs[0] == "kill" {
				return true
			}
			return len(subs) == 2 && (subs[1] == "rm" || subs[1] == "prune")
		}},
		legacyToolSubRule("kubectl-delete", "deleting kubernetes resources", "kubectl", "delete", "drain", "cordon"),
		legacyToolSubRule("terraform-destroy", "destroying infrastructure", "terraform", "destroy"),
		legacyToolSubRule("tofu-destroy", "destroying infrastructure", "tofu", "destroy"),
		{ID: "cloud-delete", Description: "deleting cloud resources", Mode: ModeDeny, Match: func(call shellCall) bool {
			switch call.Name {
			case "aws", "gcloud", "az", "doctl", "gsutil":
			default:
				return false
			}
			for _, arg := range call.Args {
				if arg == "rm" || arg == "delete" || arg == "destroy" || arg == "terminate" ||
					strings.HasPrefix(arg, "delete-") || strings.HasPrefix(arg, "terminate-") || strings.HasPrefix(arg, "remove-") {
					return true
				}
			}
			return false
		}},
		legacyToolSubRule("package-publish", "publishing or unpublishing a package", "npm", "publish", "unpublish", "deprecate"),
		legacyToolSubRule("package-publish-yarn", "publishing a package", "yarn", "publish"),
		legacyToolSubRule("package-publish-pnpm", "publishing a package", "pnpm", "publish"),
		legacyToolSubRule("cargo-publish", "publishing a crate", "cargo", "publish", "yank"),
		legacyNameRule("twine", "publishing a python package", "twine"),
	}
}
