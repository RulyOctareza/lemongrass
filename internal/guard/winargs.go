// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// Windows tools take options as /x, -x or, written in Git Bash so MSYS does not turn them into a path, //x, and compare them without case. PowerShell parameters are -Name, also abbreviated to any unique prefix.

// winOption returns the first argument that is one of names as a Windows option, such as /S, //s or -s for "s", optionally followed by :value.
func winOption(args []string, names ...string) string {
	for _, arg := range args {
		word := strings.ToLower(arg)
		switch {
		case strings.HasPrefix(word, "//"):
			word = word[2:]
		case strings.HasPrefix(word, "/") || strings.HasPrefix(word, "-"):
			word = word[1:]
		default:
			continue
		}
		if colon := strings.IndexByte(word, ':'); colon > 0 {
			word = word[:colon]
		}
		for _, name := range names {
			if word == name {
				return arg
			}
		}
	}
	return ""
}

// psParameter returns the first argument that names the PowerShell parameter name, written whole or as a prefix of at least minimum letters, in any case.
func psParameter(args []string, name string, minimum int) string {
	for _, arg := range args {
		word := strings.ToLower(arg)
		if !strings.HasPrefix(word, "-") {
			continue
		}
		word = word[1:]
		if colon := strings.IndexByte(word, ':'); colon > 0 {
			word = word[:colon]
		}
		if len(word) >= minimum && strings.HasPrefix(name, word) {
			return arg
		}
	}
	return ""
}

// winOptionForm matches a call carrying one of names as a Windows option.
func winOptionForm(names ...string) func(shellCall, matchEnv) (string, bool) {
	return func(call shellCall, _ matchEnv) (string, bool) {
		if arg := winOption(call.Args, names...); arg != "" {
			return "Matched: " + call.Name + " with " + arg + ".", true
		}
		return "", false
	}
}

// firstWordIn matches a call whose first positional, without case, is one of words, as in reg add or sc stop.
func firstWordIn(words ...string) func(shellCall, matchEnv) (string, bool) {
	return func(call shellCall, _ matchEnv) (string, bool) {
		positional := positionals(call)
		if len(positional) == 0 {
			return "", false
		}
		first := strings.ToLower(positional[0])
		for _, word := range words {
			if first == word {
				return "Matched: " + call.Name + " " + positional[0] + ".", true
			}
		}
		return "", false
	}
}

// opaqueShell matches cmd, PowerShell and the script hosts, whose command text the POSIX parser cannot read, unless the only argument asks for the version or help.
func opaqueShell(call shellCall, _ matchEnv) (string, bool) {
	if len(call.Args) == 1 {
		switch strings.ToLower(call.Args[0]) {
		case "--version", "-version", "-v", "/?", "//?", "-?", "-help", "--help":
			return "", false
		}
	}
	return "Matched: " + call.Name + ", whose command text the guard cannot read. Run the command in bash directly.", true
}
