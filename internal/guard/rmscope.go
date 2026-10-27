// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package guard

import (
	"path/filepath"
	"strings"
)

var deletionScopeRules = map[string]bool{"rm-force": true, "rm-plain": true, "find-delete": true}

var directoryChangers = map[string]bool{"cd": true, "pushd": true, "popd": true}

// deletionDirs returns every directory the script's deletions could run from, or why deletions in it cannot be judged.
func (c checker) deletionDirs(script shellScript) ([]string, string) {
	root := c.projectRoot()
	if root == "" {
		return nil, "This session has no launch project (a Codex tab or a shell outside lemongrass), so every deletion needs the user."
	}
	if c.in.Cwd == "" {
		return nil, "The shell's current directory is unknown, so the paths cannot be checked."
	}
	start := resolveFully(c.in.Cwd)
	if !withinOrEqual(start, root) {
		return nil, "The shell is in " + start + ", outside the project " + root + ". cd back into the project in its own call first."
	}
	dirs := []string{start}
	for _, call := range script.Calls {
		if !directoryChangers[call.Name] {
			continue
		}
		shown := strings.TrimSpace(call.Name + " " + strings.Join(call.Args, " "))
		target, ok := directoryTarget(call)
		if !ok {
			return nil, "'" + shown + "' moves to a directory the guard cannot know, so the paths after it cannot be checked. Write the paths from the project instead."
		}
		if !literalTarget(target) {
			return nil, "'" + shown + "' uses a variable or substitution, so the paths after it cannot be checked. Write the directory out."
		}
		for _, dir := range dirs {
			located := ResolvePath(target, dir, c.in.Home)
			next := resolveFully(located)
			if !withinOrEqual(next, root) {
				if next != located {
					return nil, "'" + shown + "' is a symlink to " + next + ", outside the project " + root + "."
				}
				return nil, "'" + shown + "' leaves the project (" + next + "), so the paths after it cannot be checked. Write the paths from inside the project instead."
			}
			if !containsArg(dirs, next) {
				dirs = append(dirs, next)
			}
		}
	}
	return dirs, ""
}

func directoryTarget(call shellCall) (string, bool) {
	if call.Name == "popd" {
		return "", false
	}
	for i, arg := range call.Args {
		if arg == "--" {
			if i+1 < len(call.Args) {
				return call.Args[i+1], true
			}
			return "", false
		}
		if arg == "-L" || arg == "-P" || arg == "-e" || arg == "-@" {
			continue
		}
		if arg == "-" || strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, "-") {
			return "", false
		}
		return arg, true
	}
	return "", false
}

// deletionCheck returns why an rm or find -delete call does not only remove paths inside the project root and outside its protected directories, or "" when it does.
func (c checker) deletionCheck(call shellCall, dirs []string) string {
	if call.Indirect {
		return "The paths come from xargs, find -exec or env -C at run time, so they cannot be checked. Name the files on the rm itself."
	}
	switch call.Name {
	case "rm":
		if hasLongFlag(call.Args, "--no-preserve-root") {
			return "--no-preserve-root is never allowed."
		}
		targets := rmTargets(call.Args)
		if len(targets) == 0 {
			return "rm has no path to check."
		}
		return c.targetsCheck(targets, dirs, false)
	case "find":
		roots, followsLinks := findRoots(call.Args)
		if followsLinks {
			return "find follows symlinks with -L, -H or -follow, which can reach outside the project. Drop that option."
		}
		return c.targetsCheck(roots, dirs, true)
	}
	return "Only rm and find -delete can be judged."
}

func (c checker) targetsCheck(targets, dirs []string, isFind bool) string {
	for _, target := range targets {
		if !literalTarget(target) {
			return "'" + target + "' uses a variable, substitution, braces or ~user, so where it lands is unknown until it runs. Write the path out."
		}
		for _, dir := range dirs {
			resolved := []string{ResolvePath(target, dir, c.in.Home)}
			if strings.ContainsAny(target, "*?[") {
				matches, err := filepath.Glob(resolved[0])
				if err != nil {
					return "'" + target + "' is not a valid pattern."
				}
				resolved = append(resolved, matches...)
			}
			for _, p := range resolved {
				location := filepath.Join(resolveExisting(filepath.Dir(p)), filepath.Base(p))
				for _, candidate := range []string{location, resolveFully(p)} {
					if reason := c.candidateCheck(target, candidate, dir, dirs, isFind); reason != "" {
						return reason
					}
				}
			}
		}
	}
	return ""
}

func (c checker) candidateCheck(target, candidate, dir string, dirs []string, isFind bool) string {
	root := c.projectRoot()
	subject := "'" + target + "'"
	if candidate != target {
		subject += " resolves to " + candidate + " and"
	}
	if candidate == root {
		if isFind {
			return "find from the project root would also search .git and biblio. Search a subfolder instead."
		}
		return subject + " is the project root, which cannot be removed."
	}
	if !strictlyWithin(candidate, root) {
		from := ""
		if dir != dirs[0] {
			from = " when run from " + dir
		}
		return subject + " is outside the project " + root + from + ". Only paths inside the project can be removed."
	}
	if why := protectedReason(candidate, root); why != "" {
		return "'" + target + "' " + why
	}
	return ""
}

func (c checker) projectRoot() string {
	if c.in.ProjectRoot == "" {
		return ""
	}
	return resolveFully(c.in.ProjectRoot)
}

func rmTargets(args []string) []string {
	var targets []string
	flagsDone := false
	for _, arg := range args {
		if !flagsDone && arg == "--" {
			flagsDone = true
			continue
		}
		if !flagsDone && strings.HasPrefix(arg, "-") && len(arg) > 1 {
			continue
		}
		targets = append(targets, arg)
	}
	return targets
}

func findRoots(args []string) (roots []string, followsLinks bool) {
	i := 0
	for ; i < len(args); i++ {
		arg := args[i]
		if arg == "-L" || arg == "-H" {
			followsLinks = true
		} else if arg == "-D" {
			i++
		} else if arg != "-P" && !strings.HasPrefix(arg, "-O") {
			break
		}
	}
	for ; i < len(args); i++ {
		arg := args[i]
		if strings.HasPrefix(arg, "-") || arg == "(" || arg == "!" || arg == "," || arg == ")" {
			break
		}
		roots = append(roots, arg)
	}
	if containsArg(args, "-follow") {
		followsLinks = true
	}
	if len(roots) == 0 {
		roots = []string{"."}
	}
	return roots, followsLinks
}

func literalTarget(target string) bool {
	if strings.HasPrefix(target, "~") && target != "~" && !strings.HasPrefix(target, "~/") {
		return false
	}
	rest := target
	for _, prefix := range []string{"$HOME", "${HOME}"} {
		if strings.HasPrefix(rest, prefix) {
			rest = strings.TrimPrefix(rest, prefix)
			break
		}
	}
	return !strings.ContainsAny(rest, "${}`")
}

func protectedReason(p, root string) string {
	scratchpad := filepath.Join(root, "biblio", "scratchpad")
	archive := filepath.Join(scratchpad, "archive")
	switch {
	case withinOrEqual(p, filepath.Join(root, ".git")):
		return "is in .git, which is protected."
	case withinOrEqual(p, archive):
		return "is in biblio/scratchpad/archive, the frozen record of closed tasks, which is protected."
	case p == scratchpad:
		return "is biblio/scratchpad itself, which is protected. Remove a task folder inside it instead."
	case strictlyWithin(p, scratchpad):
		return ""
	case withinOrEqual(p, filepath.Join(root, "biblio")):
		return "is in biblio, which is protected. Only biblio/scratchpad outside its archive can be removed."
	}
	return ""
}

func resolveFully(p string) string {
	if resolved, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(resolved)
	}
	return resolveExisting(p)
}

// resolveExisting resolves symlinks in the deepest existing ancestor of p and keeps the missing remainder as written.
func resolveExisting(p string) string {
	dir, rest := filepath.Clean(p), ""
	for {
		if resolved, err := filepath.EvalSymlinks(dir); err == nil {
			return filepath.Join(resolved, rest)
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Clean(p)
		}
		rest = filepath.Join(filepath.Base(dir), rest)
		dir = parent
	}
}

func strictlyWithin(p, root string) bool {
	return strings.HasPrefix(p, strings.TrimRight(root, string(filepath.Separator))+string(filepath.Separator))
}

func withinOrEqual(p, root string) bool {
	return p == root || strictlyWithin(p, root)
}
