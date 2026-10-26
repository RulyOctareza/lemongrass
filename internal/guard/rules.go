// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package guard

import (
	"regexp"
	"strings"
)

type dangerRule struct {
	ID          string
	Description string
	Mode        Mode
	Match       func(call shellCall) bool
}

var remoteRsyncArg = regexp.MustCompile(`^[^/\s-][^/\s]*:`)

func hasShortFlag(args []string, letters string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsAny(arg[1:], letters) {
			return true
		}
	}
	return false
}

func hasLongFlag(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				return true
			}
		}
	}
	return false
}

func containsArg(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name {
				return true
			}
		}
	}
	return false
}

var gitValueOptions = map[string]bool{"-C": true, "-c": true, "--git-dir": true, "--work-tree": true, "--namespace": true, "--super-prefix": true, "--config-env": true, "--exec-path": true}

func gitSubcommand(args []string) (string, []string) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if gitValueOptions[arg] {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		return arg, args[i+1:]
	}
	return "", nil
}

func gitRule(id, description string, mode Mode, match func(sub string, rest []string) bool) dangerRule {
	return dangerRule{ID: id, Description: description, Mode: mode, Match: func(call shellCall) bool {
		if call.Name != "git" {
			return false
		}
		sub, rest := gitSubcommand(call.Args)
		return sub != "" && match(sub, rest)
	}}
}

func nameRule(id, description string, names ...string) dangerRule {
	return dangerRule{ID: id, Description: description, Mode: ModeDeny, Match: func(call shellCall) bool {
		for _, name := range names {
			if call.Name == name {
				return true
			}
		}
		return false
	}}
}

func prefixRule(id, description, prefix string) dangerRule {
	return dangerRule{ID: id, Description: description, Mode: ModeDeny, Match: func(call shellCall) bool {
		return strings.HasPrefix(call.Name, prefix)
	}}
}

func toolSubRule(id, description, tool string, subs ...string) dangerRule {
	return dangerRule{ID: id, Description: description, Mode: ModeDeny, Match: func(call shellCall) bool {
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

func curlSendsFile(args []string) bool {
	dataFlags := map[string]bool{"-d": true, "--data": true, "--data-binary": true, "--data-ascii": true, "--json": true}
	formFlags := map[string]bool{"-F": true, "--form": true}
	for i, arg := range args {
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case arg == "-T" || arg == "--upload-file":
			return true
		case strings.HasPrefix(arg, "-T") && len(arg) > 2 && !strings.HasPrefix(arg, "--"):
			return true
		case dataFlags[arg] && strings.HasPrefix(next, "@"):
			return true
		case strings.HasPrefix(arg, "-d@"):
			return true
		case formFlags[arg] && (strings.Contains(next, "=@") || strings.Contains(next, "=<")):
			return true
		case strings.HasPrefix(arg, "-F") && len(arg) > 2 && (strings.Contains(arg, "=@") || strings.Contains(arg, "=<")):
			return true
		}
	}
	return false
}

func broadRecursiveTarget(args []string, home string) bool {
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		switch strings.TrimRight(arg, "/") {
		case "", "~", "$HOME", "/*", "~/*", "$HOME/*", "/home", "/root", "/etc", "/usr", "/var", "/opt", "/boot", "/bin", "/lib", "/sbin":
			return true
		}
		if home != "" && strings.TrimRight(arg, "/") == strings.TrimRight(home, "/") {
			return true
		}
	}
	return false
}

func dangerCatalog(home string) []dangerRule {
	return []dangerRule{
		{ID: "rm-force", Description: "rm with a force or recursive flag", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "rm" && (hasShortFlag(call.Args, "frR") || hasLongFlag(call.Args, "--force", "--recursive", "--no-preserve-root"))
		}},
		{ID: "rm-plain", Description: "removing a file", Mode: ModeApproval, Match: func(call shellCall) bool {
			return call.Name == "rm"
		}},
		{ID: "find-delete", Description: "find -delete", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "find" && containsArg(call.Args, "-delete")
		}},
		nameRule("shred", "overwriting files unrecoverably", "shred", "srm", "wipe"),
		nameRule("unlink", "unlinking a file", "unlink"),

		gitRule("git-push-force", "a forced or deleting git push", ModeDeny, func(sub string, rest []string) bool {
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
		gitRule("git-reset-hard", "git reset --hard", ModeDeny, func(sub string, rest []string) bool {
			return sub == "reset" && containsArg(rest, "--hard", "--merge", "--keep")
		}),
		gitRule("git-clean", "git clean with force", ModeDeny, func(sub string, rest []string) bool {
			return sub == "clean" && (hasShortFlag(rest, "f") || hasLongFlag(rest, "--force"))
		}),
		gitRule("git-branch-delete", "git branch force delete", ModeDeny, func(sub string, rest []string) bool {
			return sub == "branch" && (hasShortFlag(rest, "D") || (containsArg(rest, "--delete") && hasLongFlag(rest, "--force")))
		}),
		gitRule("git-stash-drop", "dropping or clearing stashes", ModeDeny, func(sub string, rest []string) bool {
			return sub == "stash" && len(rest) > 0 && (rest[0] == "drop" || rest[0] == "clear")
		}),
		gitRule("git-history-rewrite", "rewriting git history", ModeDeny, func(sub string, rest []string) bool {
			return sub == "filter-branch" || sub == "filter-repo"
		}),
		gitRule("git-reflog-expire", "expiring or deleting the reflog", ModeDeny, func(sub string, rest []string) bool {
			return sub == "reflog" && len(rest) > 0 && (rest[0] == "expire" || rest[0] == "delete")
		}),
		gitRule("git-prune", "pruning unreachable git objects", ModeDeny, func(sub string, rest []string) bool {
			return sub == "prune" || (sub == "gc" && hasLongFlag(rest, "--prune"))
		}),
		gitRule("git-add", "staging changes", ModeApproval, func(sub string, rest []string) bool {
			return sub == "add"
		}),
		gitRule("git-commit", "committing", ModeApproval, func(sub string, rest []string) bool {
			return sub == "commit"
		}),

		nameRule("disk-tool", "a raw disk or filesystem tool", "dd", "fdisk", "sfdisk", "cfdisk", "parted", "wipefs", "sgdisk", "mkswap", "cryptsetup", "losetup"),
		prefixRule("mkfs", "creating a filesystem", "mkfs"),
		nameRule("privilege", "privilege escalation", "sudo", "su", "doas", "pkexec", "run0"),
		nameRule("power", "shutting down or rebooting the machine", "shutdown", "reboot", "poweroff", "halt", "telinit"),
		nameRule("firewall", "changing the firewall", "iptables", "ip6tables", "nft", "ufw", "firewall-cmd"),
		nameRule("kill-by-name", "killing processes by name", "pkill", "killall"),
		{ID: "kill-all", Description: "killing every process", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "kill" && containsArg(call.Args, "-1")
		}},
		toolSubRule("systemctl-stop", "stopping, disabling or masking a service", "systemctl", "stop", "disable", "mask", "kill", "poweroff", "reboot", "halt", "suspend", "hibernate", "isolate"),
		{ID: "recursive-perms", Description: "recursive chmod or chown on a broad path", Mode: ModeDeny, Match: func(call shellCall) bool {
			if call.Name != "chmod" && call.Name != "chown" && call.Name != "chgrp" {
				return false
			}
			return (hasShortFlag(call.Args, "R") || hasLongFlag(call.Args, "--recursive")) && broadRecursiveTarget(call.Args, home)
		}},
		{ID: "crontab", Description: "changing the crontab", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "crontab" && !(len(call.Args) == 1 && call.Args[0] == "-l")
		}},
		nameRule("at-jobs", "scheduling jobs with at", "at", "batch"),

		{ID: "curl-upload", Description: "curl sending a local file", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "curl" && curlSendsFile(call.Args)
		}},
		{ID: "wget-upload", Description: "wget sending a local file", Mode: ModeDeny, Match: func(call shellCall) bool {
			return call.Name == "wget" && hasLongFlag(call.Args, "--post-file", "--body-file")
		}},
		nameRule("raw-network", "opening a raw network connection", "nc", "ncat", "netcat", "socat", "telnet"),
		nameRule("remote-shell", "a remote shell or file copy", "ssh", "scp", "sftp", "sshfs", "mosh"),
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

		nameRule("keyring", "reading the OS keyring", "secret-tool", "keyctl"),
		nameRule("db-client", "a direct database client, use lgrass db instead", "mysql", "mariadb", "psql", "mongosh", "mongo", "redis-cli", "sqlcmd", "pg_dump", "pg_dumpall", "pg_restore", "mysqldump", "mysqladmin", "mariadb-dump", "cockroach", "clickhouse-client"),

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
		toolSubRule("kubectl-delete", "deleting kubernetes resources", "kubectl", "delete", "drain", "cordon"),
		toolSubRule("terraform-destroy", "destroying infrastructure", "terraform", "destroy"),
		toolSubRule("tofu-destroy", "destroying infrastructure", "tofu", "destroy"),
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
		toolSubRule("package-publish", "publishing or unpublishing a package", "npm", "publish", "unpublish", "deprecate"),
		toolSubRule("package-publish-yarn", "publishing a package", "yarn", "publish"),
		toolSubRule("package-publish-pnpm", "publishing a package", "pnpm", "publish"),
		toolSubRule("cargo-publish", "publishing a crate", "cargo", "publish", "yank"),
		nameRule("twine", "publishing a python package", "twine"),
	}
}
