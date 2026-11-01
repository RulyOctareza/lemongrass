// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

var forgeTools = []string{"gh", "glab", "tea", "hub"}

var forgeRules = []rule{
	{ID: "forge-delete", Category: catForges, Description: "deleting repositories or releases, or changing secrets and keys, on GitHub or GitLab", Mode: ModeDeny, Forms: []form{
		{Tools: forgeTools, Subs: []string{"repo delete", "release delete", "release delete-asset", "issue delete", "run delete", "gist delete", "label delete", "secret set", "secret delete", "secret remove", "variable delete", "ssh-key add", "ssh-key delete", "gpg-key add", "gpg-key delete", "cache delete", "deploy-key add", "deploy-key delete", "ruleset delete", "project delete", "repo archive", "mr delete", "snippet delete", "variable remove", "token create", "token revoke"}},
		{Tools: []string{"hub"}, Subs: []string{"delete"}},
		{Tools: []string{"gh", "glab"}, Subs: []string{"api"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			if forgeAPIMethod(call) == "DELETE" {
				return "Matched: " + call.Name + " api with the DELETE method.", true
			}
			return "", false
		}},
	}},
	{ID: "forge-write", Category: catForges, Description: "creating or changing pull requests, issues, releases or repositories on GitHub or GitLab", Mode: ModeAsk, Forms: []form{
		{Tools: forgeTools, Subs: []string{
			"pr create", "pr merge", "pr close", "pr reopen", "pr edit", "pr comment", "pr review", "pr ready", "pr lock", "pr unlock", "pr revert", "pr update-branch",
			"mr create", "mr merge", "mr close", "mr reopen", "mr update", "mr note", "mr approve", "mr revoke", "mr rebase",
			"issue create", "issue close", "issue reopen", "issue edit", "issue comment", "issue transfer", "issue pin", "issue unpin", "issue lock", "issue unlock", "issue develop", "issue update", "issue note",
			"release create", "release edit", "release upload",
			"repo create", "repo edit", "repo rename", "repo fork", "repo sync", "repo set-default", "repo transfer", "repo unarchive",
			"workflow run", "workflow enable", "workflow disable", "run cancel", "run rerun",
			"gist create", "gist edit", "gist rename", "label create", "label edit", "label clone", "variable set",
			"ci run", "ci retry", "ci cancel", "ci trigger", "schedule create", "schedule run",
			"project create", "project edit", "project item-add", "project item-edit", "project field-create",
			"codespace create", "codespace delete", "codespace stop", "extension install", "extension remove", "extension upgrade",
		}},
		{Tools: []string{"tea"}, Subs: []string{"pulls create", "pulls merge", "pulls close", "issues create", "issues close", "releases create", "repos create", "comment"}},
		{Tools: []string{"hub"}, Subs: []string{"pull-request", "create", "fork", "release create", "release edit", "issue create", "issue update", "merge", "sync"}},
		{Tools: []string{"gh", "glab"}, Subs: []string{"api"}, Match: func(call shellCall, _ matchEnv) (string, bool) {
			switch method := forgeAPIMethod(call); method {
			case "", "GET", "HEAD", "DELETE":
				return "", false
			default:
				return "Matched: " + call.Name + " api with the " + method + " method.", true
			}
		}},
	}},
}

var forgeAPIFieldFlags = map[string]bool{"-f": true, "-F": true, "--field": true, "--raw-field": true, "--input": true}

// forgeAPIMethod returns the HTTP method a gh or glab api call sends. Fields make the default POST, except for a GraphQL query, which reads unless its text holds a mutation.
func forgeAPIMethod(call shellCall) string {
	method := ""
	fields := false
	mutation := false
	for i, arg := range call.Args {
		next := ""
		if i+1 < len(call.Args) {
			next = call.Args[i+1]
		}
		switch {
		case arg == "-X" || arg == "--method":
			method = strings.ToUpper(next)
		case strings.HasPrefix(arg, "--method="):
			method = strings.ToUpper(strings.TrimPrefix(arg, "--method="))
		case strings.HasPrefix(arg, "-X") && len(arg) > 2:
			method = strings.ToUpper(arg[2:])
		case forgeAPIFieldFlags[arg]:
			fields = true
			if strings.Contains(next, "mutation") {
				mutation = true
			}
		case strings.HasPrefix(arg, "--field=") || strings.HasPrefix(arg, "--raw-field=") || strings.HasPrefix(arg, "--input="):
			fields = true
			if strings.Contains(arg, "mutation") {
				mutation = true
			}
		}
	}
	if method != "" {
		return method
	}
	if !fields {
		return ""
	}
	if containsArg(positionals(call), "graphql") && !mutation {
		return ""
	}
	return "POST"
}
