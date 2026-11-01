// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

// rule is one catalog entry: what the user sees in the Safety section and what the engine matches. A rule with no forms is judged by an engine check outside the catalog loop, such as secret paths or download-exec.
type rule struct {
	ID          string
	Category    string
	Description string
	Mode        Mode
	// Locked rules protect lemongrass itself and cannot be changed by a policy.
	Locked bool
	Forms  []form
	// Except exempts a call that one of the forms matched, such as a package restore inside package-install.
	Except func(call shellCall) bool
	// WritePaths are paths whose write the rule catches, from any tool, redirect or write tool. "~/x" is under the home directory, a trailing slash means everything inside, and an entry without ~ matches as a path suffix anywhere, such as ".git/hooks/".
	WritePaths []string
}

// form is one shape of command a rule matches. Every field that is set must hold; a form with no tools applies to every call.
type form struct {
	Tools        []string
	ToolPrefixes []string
	// Subs are subcommand paths, words separated by spaces, matched against the leading positionals after the tool's global options. "*" matches any one word.
	Subs []string
	// Flags are any of: a short letter written "-f", found inside any combined short flag, or a long flag "--force", also as "--force=x".
	Flags []string
	// Args are literal arguments, any one present.
	Args []string
	// ArgPrefixes match an argument starting with one of them.
	ArgPrefixes []string
	// Unless lists arguments that exempt the call. "--pull=never" also matches the two arguments "--pull" "never".
	Unless []string
	// NoSubs names subcommand paths that exempt the call even when Subs is empty.
	NoSubs []string
	Match  func(call shellCall, env matchEnv) (string, bool)
}

// matchEnv is what a predicate may need beyond the call itself.
type matchEnv struct {
	Home string
}

// Categories as the Safety section groups them.
const (
	catDeletion       = "Deletion"
	catScripts        = "Scripts"
	catGit            = "Git"
	catForges         = "Forges"
	catCredentials    = "Credentials"
	catContainers     = "Containers"
	catSystem         = "System"
	catNetwork        = "Network"
	catDownload       = "Download"
	catPackages       = "Packages"
	catInfrastructure = "Infrastructure"
	catDatabase       = "Database"
	catLemongrass     = "Lemongrass"
)

// catalogRules is every rule in judging order. When one call matches several deny rules the first is reported, so the rules that existed before the engine come first within each section.
var catalogRules = concatRules(
	deletionRules,
	scriptRules,
	gitRules,
	systemRules,
	networkRules,
	lemongrassRules,
	databaseRules,
	containerRules,
	infrastructureRules,
	packageRules,
	forgeRules,
	credentialRules,
	downloadRules,
)

var catalogByID = indexRules(catalogRules)

func concatRules(sections ...[]rule) []rule {
	var out []rule
	for _, section := range sections {
		out = append(out, section...)
	}
	return out
}

func indexRules(rules []rule) map[string]*rule {
	index := make(map[string]*rule, len(rules))
	for i := range rules {
		index[rules[i].ID] = &rules[i]
	}
	return index
}

func isLocked(id string) bool {
	r, ok := catalogByID[id]
	return ok && r.Locked
}

// ruleVerdict is a verdict for an engine check, with the description and mode the catalog gives its id.
func ruleVerdict(id, detail string) Verdict {
	r := catalogByID[id]
	return Verdict{RuleID: id, Category: r.Category, Description: r.Description, Mode: r.Mode, Detail: detail}
}

// Catalog lists every rule the user can see at its built-in mode.
func Catalog() []RuleInfo {
	infos := make([]RuleInfo, 0, len(catalogRules))
	for _, r := range catalogRules {
		infos = append(infos, RuleInfo{ID: r.ID, Category: r.Category, Description: r.Description, Mode: modeName(r.Mode), Locked: r.Locked})
	}
	return infos
}

func knownRules() map[string]RuleInfo {
	known := map[string]RuleInfo{}
	for _, info := range Catalog() {
		known[info.ID] = info
	}
	return known
}
