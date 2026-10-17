// Package how serves the step-by-step instructions behind `lgrass how <domain> <action>`, printing only the branch that applies.
package how

import (
	"embed"
	"fmt"
	"sort"
	"strings"
)

//go:embed content
var content embed.FS

type action struct {
	summary string
	run     func(root string, args []string) (string, error)
}

var domains = map[string]map[string]action{
	"collaborator": {
		"join": {summary: "What to do after the human adds your tab to a workgroup: `lgrass how collaborator join`", run: static("collaborator/join.md")},
	},
	"leader": {
		"addmember": {summary: "Add thinkers to your live workgroup: `lgrass how leader addmember`", run: static("leader/addmember.md")},
	},
	"task": {
		"close": {summary: "Close a finished scratchpad task: `lgrass how task close <slug>`", run: taskClose},
	},
}

// static serves an action that has one branch and takes no arguments.
func static(path string) func(root string, args []string) (string, error) {
	return func(_ string, args []string) (string, error) {
		if len(args) > 0 {
			return "", fmt.Errorf("how: %s takes no arguments", strings.TrimSuffix(path, ".md"))
		}
		return fragment(path, nil) + "\n", nil
	}
}

// Render resolves args to a domain listing, an action listing, or an action's instructions for the project at root.
func Render(root string, args []string) (string, error) {
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
	return act.run(root, args[2:])
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
