// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"path/filepath"
	"regexp"
	"strings"
)

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

// argsAfter returns the call's arguments after its n-th positional, counted as positionals counts them, or nil when it has fewer.
func argsAfter(call shellCall, n int) []string {
	values := toolValueOptions[call.Name]
	seen := 0
	for i := 0; i < len(call.Args); i++ {
		arg := call.Args[i]
		if arg == "--" {
			return nil
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			if values[arg] {
				i++
			}
			continue
		}
		seen++
		if seen == n {
			return call.Args[i+1:]
		}
	}
	return nil
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
		if home != "" && (strings.TrimRight(arg, "/") == strings.TrimRight(home, "/") || filepath.Clean(arg) == ResolveDir(home)) {
			return true
		}
	}
	return false
}

// matched builds the predicate result for a form whose own detail is enough.
func matched(ok bool) (string, bool) {
	return "", ok
}
