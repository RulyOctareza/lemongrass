// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"os"
	"path/filepath"
	"strings"
)

var npmInstallAliases = map[string]bool{"install": true, "i": true, "in": true, "ins": true, "inst": true, "insta": true, "instal": true, "isnt": true, "isnta": true, "isntal": true, "isntall": true}

var npmCleanInstallAliases = map[string]bool{"ci": true, "clean-install": true, "install-clean": true, "isntall-clean": true, "cit": true, "install-ci-test": true}

// restoreCall reports a call that fetches only the dependencies the project's manifest or lockfile declares, or runs a tool already installed in node_modules. Any package argument makes it an install.
func restoreCall(call shellCall, env matchEnv) bool {
	if args, ok := runnerArgs(call); ok {
		return localToolRun(args, env)
	}
	words := positionals(call)
	sub := ""
	if len(words) > 0 {
		sub = words[0]
	}
	global := hasShortFlag(call.Args, "g") || hasLongFlag(call.Args, "--global")
	switch call.Name {
	case "npm":
		return npmCleanInstallAliases[sub] || (npmInstallAliases[sub] && len(words) == 1 && !global)
	case "pnpm", "bun":
		return (sub == "install" || sub == "i") && len(words) == 1 && !global
	case "yarn":
		if containsArg(call.Args, "--version", "-v", "--help", "-h") {
			return false
		}
		return len(words) == 0 || (sub == "install" && len(words) == 1)
	case "deno":
		return sub == "install" && len(words) == 1 && !global
	case "pip", "pip3":
		return sub == "install" && pipRestore(argsAfter(call, 1))
	case "uv":
		return sub == "sync"
	case "poetry":
		return sub == "install"
	case "pdm":
		return sub == "install" || sub == "sync"
	case "conda", "mamba", "micromamba":
		return len(words) >= 2 && sub == "env" && words[1] == "create" && (hasShortFlag(call.Args, "f") || hasLongFlag(call.Args, "--file"))
	case "cargo":
		return sub == "fetch"
	case "go":
		return len(words) == 2 && sub == "mod" && words[1] == "download"
	case "bundle", "bundler", "composer", "pod":
		return sub == "install" && len(words) == 1
	case "dotnet":
		return sub == "restore"
	case "dart", "flutter":
		return len(words) == 2 && sub == "pub" && words[1] == "get"
	case "swift":
		return len(words) == 2 && sub == "package" && words[1] == "resolve"
	case "mix":
		return sub == "deps.get"
	}
	if strings.HasPrefix(call.Name, "python") {
		for i := 0; i+2 < len(call.Args); i++ {
			if call.Args[i] == "-m" && (call.Args[i+1] == "pip" || call.Args[i+1] == "pip3") {
				return call.Args[i+2] == "install" && pipRestore(call.Args[i+3:])
			}
		}
	}
	return false
}

var pipFileFlags = map[string]bool{"-r": true, "--requirement": true, "-c": true, "--constraint": true}

var pipEditableFlags = map[string]bool{"-e": true, "--editable": true}

var pipValueFlags = map[string]bool{
	"-i": true, "--index-url": true, "--extra-index-url": true, "-f": true, "--find-links": true, "-t": true, "--target": true,
	"--prefix": true, "--root": true, "--python-version": true, "--platform": true, "--implementation": true, "--abi": true,
	"--upgrade-strategy": true, "--src": true, "--cache-dir": true, "--log": true, "--proxy": true, "--timeout": true,
	"--retries": true, "--progress-bar": true, "--trusted-host": true, "--config-settings": true, "-C": true, "--python": true,
}

// pipRestore reports pip install arguments that name only requirement files and local paths, such as -r requirements.txt or -e .
func pipRestore(args []string) bool {
	found := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		next := ""
		if i+1 < len(args) {
			next = args[i+1]
		}
		switch {
		case pipFileFlags[arg]:
			if next == "" {
				return false
			}
			found = true
			i++
		case pipEditableFlags[arg]:
			if !localPath(next) {
				return false
			}
			found = true
			i++
		case pipValueFlags[arg]:
			i++
		case strings.HasPrefix(arg, "--requirement=") || strings.HasPrefix(arg, "--constraint=") || (strings.HasPrefix(arg, "-r") && len(arg) > 2):
			found = true
		case strings.HasPrefix(arg, "--editable="):
			if !localPath(strings.TrimPrefix(arg, "--editable=")) {
				return false
			}
			found = true
		case strings.HasPrefix(arg, "-"):
		default:
			if !localPath(arg) {
				return false
			}
			found = true
		}
	}
	return found
}

func localPath(arg string) bool {
	return strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "/")
}

// runnerArgs returns the arguments after a package runner such as npx or npm exec, and whether the call is one.
func runnerArgs(call shellCall) ([]string, bool) {
	switch call.Name {
	case "npx", "pnpx", "bunx":
		return call.Args, true
	}
	words := positionals(call)
	if len(words) == 0 {
		return nil, false
	}
	switch {
	case call.Name == "npm" && (words[0] == "exec" || words[0] == "x"),
		call.Name == "pnpm" && (words[0] == "exec"),
		call.Name == "bun" && words[0] == "x",
		call.Name == "yarn" && (words[0] == "run" || words[0] == "exec"):
		return argsAfter(call, 1), true
	}
	return nil, false
}

var runnerPackageFlags = []string{"-p", "--package", "-c", "--call"}

// localToolRun reports a runner call whose tool is installed in node_modules under the cwd or the launch project. A --package or --call option, a versioned name or a path names something to fetch.
func localToolRun(args []string, env matchEnv) bool {
	tool := ""
	for i, arg := range args {
		if arg == "--" {
			if i+1 < len(args) {
				tool = args[i+1]
			}
			break
		}
		if strings.HasPrefix(arg, "-") {
			for _, flag := range runnerPackageFlags {
				if arg == flag || strings.HasPrefix(arg, flag+"=") {
					return false
				}
			}
			continue
		}
		tool = arg
		break
	}
	if tool == "" || strings.HasPrefix(tool, ".") || strings.HasPrefix(tool, "/") {
		return false
	}
	installed := filepath.Join("node_modules", ".bin", tool)
	if strings.HasPrefix(tool, "@") {
		// A scoped package's bin name can differ from its package name, so the package directory counts.
		if strings.Count(tool, "@") > 1 || strings.Count(tool, "/") != 1 {
			return false
		}
		installed = filepath.Join("node_modules", tool)
	} else if strings.ContainsAny(tool, "@/") {
		return false
	}
	for _, dir := range []string{env.Cwd, env.ProjectRoot} {
		if dir == "" {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, installed)); err == nil {
			return true
		}
	}
	return false
}
