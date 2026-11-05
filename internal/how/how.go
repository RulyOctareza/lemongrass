// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Package how serves the step-by-step instructions behind `lgrass how <domain> <action>`, printing only the branch that applies.
package how

import (
	"embed"
	"fmt"
	"sort"
	"strings"
	"time"
)

//go:embed content
var content embed.FS

type action struct {
	summary string
	run     func(root string, args []string) (string, error)
	// runEnv replaces run for an action that reads beyond the project's files.
	runEnv func(env Env, args []string) (string, error)
}

// Env is what an action may read: the project root, and sources outside the project's files. A nil source is
// unavailable, and the facts that depend on it fall back to what the files say.
type Env struct {
	Root string
	// TaskStarted reports when biblio/scratchpad/<slug> first appeared in the biblio history.
	TaskStarted func(slug string) (time.Time, bool)
	// EditsSince lists the file edits agent sessions logged in this project after since, oldest first.
	EditsSince func(since time.Time) []Edit
}

// Edit is one file edit an agent session made, with the path as the agent's tool was given it.
type Edit struct {
	Session string
	Path    string
}

var domains = map[string]map[string]action{
	"ask": {
		"request": {summary: "Ask the human for a db or rester channel or a command run: `lgrass how ask request`", run: static("ask/request.md")},
	},
	"book": {
		"write": {summary: "Decide where a chapter goes and how to write it: `lgrass how book write`", run: static("book/where.md", "book/rules.md")},
	},
	"collaborator": {
		"join": {summary: "What to do after the human adds your tab to a workgroup: `lgrass how collaborator join`", run: static("collaborator/join.md")},
	},
	"leader": {
		"addmember": {summary: "Add thinkers to your live workgroup: `lgrass how leader addmember`", run: static("leader/addmember.md")},
		"create":    {summary: "Declare and lead a workgroup: `lgrass how leader create`", run: static("leader/create.md")},
	},
	"git": {
		"resolve": {summary: "Resolve a conflict the human handed you: `lgrass how git resolve`", run: static("git/resolve.md")},
	},
	"keeper": {
		"sweep": {summary: "Keep biblio/ and the code in agreement: `lgrass how keeper sweep`", run: keeperSweep},
	},
	"laws": {
		"write": {summary: "Write a law the user approved: `lgrass how laws write`", run: static("laws/write.md")},
	},
	"task": {
		"close":    {summary: "Close a finished scratchpad task: `lgrass how task close <slug>`", runEnv: taskClose},
		"handover": {summary: "Pause or hand over a task: `lgrass how task handover <slug>`", run: taskHandover},
	},
	"work": {
		"debug":      {summary: "Chase a fault to its root cause: `lgrass how work debug <slug>`", run: workDebug},
		"discussion": {summary: "An idea, a question or a design talk with no task file yet: `lgrass how work discussion`", run: workDiscussion},
		"research":   {summary: "Investigate with no build: `lgrass how work research <slug>`", run: workResearch},
		"task":       {summary: "Build something, new or resumed: `lgrass how work task <slug>`", run: workTask},
	},
	"thinker": {
		"start": {summary: "Your role as a thinker in a workgroup: `lgrass how thinker start`", run: static("thinker/start.md")},
	},
}

// static serves a manual: fixed text with one branch and no arguments, joined from its fragments in order.
func static(paths ...string) func(root string, args []string) (string, error) {
	return func(_ string, args []string) (string, error) {
		if len(args) > 0 {
			return "", fmt.Errorf("how: %s takes no arguments", strings.TrimSuffix(paths[0], ".md"))
		}
		parts := make([]string, len(paths))
		for i, p := range paths {
			parts[i] = fragment(p, nil)
		}
		return strings.Join(parts, "\n\n") + "\n", nil
	}
}

// Render resolves args to a domain listing, an action listing, or an action's instructions for the project at root.
func Render(root string, args []string) (string, error) {
	return RenderEnv(Env{Root: root}, args)
}

// RenderEnv is Render with the sources outside the project's files that some actions use.
func RenderEnv(env Env, args []string) (string, error) {
	if len(args) == 0 {
		return listDomains(), nil
	}
	actions, ok := domains[args[0]]
	if !ok {
		return "", fmt.Errorf("how: unknown domain %q\n\n%s", args[0], listDomains())
	}
	if len(args) == 1 {
		return listActions(args[0], actions), nil
	}
	act, ok := actions[args[1]]
	if !ok {
		return "", fmt.Errorf("how: unknown action %q in %s\n\n%s", args[1], args[0], listActions(args[0], actions))
	}
	if act.runEnv != nil {
		return act.runEnv(env, args[2:])
	}
	return act.run(env.Root, args[2:])
}

// Known reports whether domain and action name an existing action, without running it.
func Known(domain, actionName string) bool {
	_, ok := domains[domain][actionName]
	return ok
}

func listDomains() string {
	var b strings.Builder
	b.WriteString("Domains, each listing its actions with `lgrass how <domain>`:\n")
	for _, name := range sortedKeys(domains) {
		fmt.Fprintf(&b, "  %s\n", name)
	}
	return b.String()
}

func listActions(domain string, actions map[string]action) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Actions in %s:\n", domain)
	for _, name := range sortedKeys(actions) {
		fmt.Fprintf(&b, "  %s  %s\n", name, actions[name].summary)
	}
	return b.String()
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func fragment(path string, vars map[string]string) string {
	data, err := content.ReadFile("content/" + path)
	if err != nil {
		panic("how: missing embedded fragment " + path)
	}
	text := strings.TrimSpace(string(data))
	for k, v := range vars {
		text = strings.ReplaceAll(text, "{"+k+"}", v)
	}
	return text
}
