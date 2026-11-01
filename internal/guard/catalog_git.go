// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

var gitTool = []string{"git"}

var gitRules = []rule{
	{ID: "git-push-force", Category: catGit, Description: "a forced or deleting git push", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"push"}, Match: gitPushForce},
	}},
	{ID: "git-reset-hard", Category: catGit, Description: "git reset --hard", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"reset"}, Args: []string{"--hard", "--merge", "--keep"}},
	}},
	{ID: "git-clean", Category: catGit, Description: "git clean with force", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"clean"}, Flags: []string{"-f", "--force"}},
	}},
	{ID: "git-branch-delete", Category: catGit, Description: "git branch force delete", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"branch"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			rest := afterGitSub(call)
			return matched(hasShortFlag(rest, "D") || (containsArg(rest, "--delete") && hasLongFlag(rest, "--force")))
		}},
	}},
	{ID: "git-stash-drop", Category: catGit, Description: "dropping or clearing stashes", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"stash drop", "stash clear"}},
	}},
	{ID: "git-history-rewrite", Category: catGit, Description: "rewriting git history", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"filter-branch", "filter-repo"}},
	}},
	{ID: "git-reflog-expire", Category: catGit, Description: "expiring or deleting the reflog", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"reflog expire", "reflog delete"}},
	}},
	{ID: "git-prune", Category: catGit, Description: "pruning unreachable git objects", Mode: ModeDeny, Forms: []form{
		{Tools: gitTool, Subs: []string{"prune", "lfs prune"}},
		{Tools: gitTool, Subs: []string{"gc"}, Flags: []string{"--prune"}},
	}},
	{ID: "git-add", Category: catGit, Description: "staging changes", Mode: ModeAsk, Forms: []form{{Tools: gitTool, Subs: []string{"add"}}}},
	{ID: "git-commit", Category: catGit, Description: "committing", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"commit", "cherry-pick", "revert", "am"}},
	}},

	{ID: "git-push", Category: catGit, Description: "pushing to a remote", Mode: ModeAsk, Forms: []form{{Tools: gitTool, Subs: []string{"push"}}}},
	{ID: "git-merge", Category: catGit, Description: "merging branches", Mode: ModeAsk, Forms: []form{{Tools: gitTool, Subs: []string{"merge"}}}},
	{ID: "git-rebase", Category: catGit, Description: "rebasing", Mode: ModeAsk, Forms: []form{{Tools: gitTool, Subs: []string{"rebase"}}}},
	{ID: "git-tag", Category: catGit, Description: "creating or deleting a tag", Mode: ModeAsk, Forms: []form{{Tools: gitTool, Subs: []string{"tag"}, Match: gitTagWrite}}},
	{ID: "git-stash", Category: catGit, Description: "stashing, popping or applying changes", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"stash"}, NoSubs: []string{"stash list", "stash show"}},
	}},
	{ID: "git-discard", Category: catGit, Description: "discarding uncommitted changes", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"checkout"}, Match: gitCheckoutDiscards},
		{Tools: gitTool, Subs: []string{"restore"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			rest := afterGitSub(call)
			stagedOnly := (hasLongFlag(rest, "--staged") || hasShortFlag(rest, "S")) && !hasLongFlag(rest, "--worktree") && !hasShortFlag(rest, "W")
			return matched(!stagedOnly)
		}},
		{Tools: gitTool, Subs: []string{"switch"}, Flags: []string{"-f", "--force", "--discard-changes"}},
		{Tools: gitTool, Subs: []string{"worktree remove", "submodule deinit"}, Flags: []string{"-f", "--force"}},
		{Tools: gitTool, Subs: []string{"update-ref"}, Flags: []string{"-d"}},
	}},
	{ID: "git-remote-write", Category: catGit, Description: "adding, removing or changing a git remote", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"remote add", "remote remove", "remote rm", "remote rename", "remote set-url", "remote set-head", "remote set-branches", "remote prune"}},
	}},
	{ID: "git-config-write", Category: catGit, Description: "changing global git config or config that runs commands", Mode: ModeAsk, Forms: []form{
		{Tools: gitTool, Subs: []string{"config"}, Match: gitConfigWrite},
	}},
}

func afterGitSub(call shellCall) []string {
	_, rest := gitSubcommand(call.Args)
	return rest
}

func gitPushForce(call shellCall, _ matchEnv) (string, bool) {
	rest := afterGitSub(call)
	if flag := flagMatch([]string{"-f", "-d", "--force", "--force-with-lease", "--force-if-includes", "--mirror", "--delete", "--prune"}, rest); flag != "" {
		return "Matched: git push with " + flag + ".", true
	}
	for _, arg := range rest {
		if strings.HasPrefix(arg, "+") || strings.HasPrefix(arg, ":") {
			return "Matched: git push with the refspec " + arg + ", which forces or deletes a remote ref.", true
		}
	}
	return "", false
}

var gitTagListFlags = []string{"-l", "--list", "-n", "--contains", "--no-contains", "--points-at", "--merged", "--no-merged", "--sort", "--format", "--column", "-v", "--verify"}

// gitTagWrite passes the list and verify forms of git tag.
func gitTagWrite(call shellCall, _ matchEnv) (string, bool) {
	rest := afterGitSub(call)
	if hasShortFlag(rest, "d") || hasLongFlag(rest, "--delete") {
		return "Matched: git tag with --delete.", true
	}
	for _, flag := range gitTagListFlags {
		if hasLongFlag(rest, flag) || (len(flag) == 2 && hasShortFlag(rest, flag[1:])) {
			return "", false
		}
	}
	for _, arg := range rest {
		if !strings.HasPrefix(arg, "-") {
			return "", true
		}
	}
	return "", false
}

func gitCheckoutDiscards(call shellCall, _ matchEnv) (string, bool) {
	rest := afterGitSub(call)
	if hasShortFlag(rest, "f") || hasLongFlag(rest, "--force") {
		return "Matched: git checkout with --force.", true
	}
	if containsArg(rest, "--") {
		return "Matched: git checkout with paths after --, which overwrites them.", true
	}
	if containsArg(rest, ".") {
		return "Matched: git checkout ., which overwrites every changed file.", true
	}
	return "", false
}

var gitConfigReadFlags = []string{"--get", "--get-all", "--get-regexp", "--get-urlmatch", "--get-color", "--get-colorbool", "--list", "-l", "--show-origin", "--show-scope", "--name-only"}

var gitConfigWriteFlags = []string{"--unset", "--unset-all", "--replace-all", "--add", "--rename-section", "--remove-section", "--edit", "-e"}

var gitConfigScopeFlags = []string{"--global", "--system", "--file", "-f", "--blob"}

var gitConfigValueFlags = map[string]bool{"--file": true, "-f": true, "--blob": true, "--type": true, "--default": true, "--comment": true, "--value": true}

var gitConfigExecKeys = []string{"core.hookspath", "core.sshcommand", "core.fsmonitor", "core.editor", "core.pager", "sequence.editor", "gpg.program", "diff.external"}

var gitConfigExecPrefixes = []string{"credential.", "url.", "alias.", "filter."}

var gitConfigExecSuffixes = []string{".textconv", ".driver", ".command", ".cmd", ".helper"}

// gitConfigWrite matches a git config call that sets or unsets a value either in a global scope or on a key that makes git run a command.
func gitConfigWrite(call shellCall, _ matchEnv) (string, bool) {
	rest := afterGitSub(call)
	var words []string
	for i := 0; i < len(rest); i++ {
		arg := rest[i]
		if gitConfigValueFlags[arg] {
			i++
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			words = append(words, arg)
		}
	}
	writes := false
	switch {
	case containsArg(rest, gitConfigReadFlags...):
		return "", false
	case len(words) > 0 && (words[0] == "get" || words[0] == "list"):
		return "", false
	case len(words) > 0 && (words[0] == "set" || words[0] == "unset" || words[0] == "edit" || words[0] == "rename-section" || words[0] == "remove-section"):
		writes = true
		words = words[1:]
	case hasLongFlag(rest, gitConfigWriteFlags...) || containsArg(rest, "-e"):
		writes = true
	case len(words) >= 2:
		writes = true
	}
	if !writes {
		return "", false
	}
	if hasLongFlag(rest, gitConfigScopeFlags...) {
		return "Matched: git config writing outside this repository.", true
	}
	if len(words) == 0 {
		return "", false
	}
	key := strings.ToLower(words[0])
	for _, exec := range gitConfigExecKeys {
		if key == exec {
			return "Matched: git config setting " + words[0] + ", which makes git run a command.", true
		}
	}
	for _, prefix := range gitConfigExecPrefixes {
		if strings.HasPrefix(key, prefix) {
			return "Matched: git config setting " + words[0] + ", which can make git run a command or reach a credential.", true
		}
	}
	for _, suffix := range gitConfigExecSuffixes {
		if strings.HasSuffix(key, suffix) {
			return "Matched: git config setting " + words[0] + ", which makes git run a command.", true
		}
	}
	return "", false
}
