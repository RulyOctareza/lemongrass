// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "strings"

// toolValueOptions are each tool's global options that take a separate value, so the value is not read as its subcommand. A missing option only makes a subcommand later in the arguments go unseen by Subs.
var toolValueOptions = map[string]map[string]bool{
	"git":       gitValueOptions,
	"docker":    flagSet("-H", "--host", "-c", "--context", "--config", "-l", "--log-level", "-f", "--file", "-p", "--project-name", "--profile", "--env-file", "--project-directory"),
	"podman":    flagSet("--connection", "-c", "--url", "--root", "--runroot", "--storage-driver", "--log-level", "--identity", "-f", "--file", "-p", "--project-name"),
	"nerdctl":   flagSet("-H", "--address", "-n", "--namespace", "--snapshotter", "--cni-path", "-f", "--file", "-p", "--project-name"),
	"kubectl":   flagSet("-n", "--namespace", "--context", "--kubeconfig", "--cluster", "--user", "-s", "--server", "--token", "--as", "--as-group", "-o", "--output", "-l", "--selector", "-f", "--filename", "-c", "--container"),
	"helm":      flagSet("-n", "--namespace", "--kube-context", "--kubeconfig", "--repository-config", "--repository-cache", "--registry-config"),
	"systemctl": flagSet("-H", "--host", "-M", "--machine", "-t", "--type", "-p", "--property", "-s", "--signal", "-n", "--lines", "-o", "--output", "--state", "--root", "--kill-whom", "--job-mode"),
	"aws":       flagSet("--profile", "--region", "--output", "--endpoint-url", "--query", "--ca-bundle", "--color", "--cli-read-timeout", "--cli-connect-timeout"),
	"gcloud":    flagSet("--project", "--account", "--configuration", "--format", "--verbosity", "--impersonate-service-account", "--billing-project", "--region", "--zone"),
	"az":        flagSet("--subscription", "-o", "--output", "--query", "-g", "--resource-group", "-n", "--name", "-l", "--location"),
	"ip":        flagSet("-n", "-netns", "-f", "-family", "-b", "-batch", "-rc", "-rcvbuf"),
	"mount":     flagSet("-t", "--types", "-o", "--options", "-O", "--test-opts", "-L", "--label", "-U", "--uuid", "-T", "--fstab", "-N", "--namespace"),
	"gh":        flagSet("-R", "--repo", "--hostname"),
	"glab":      flagSet("-R", "--repo"),
	"terraform": flagSet("-chdir"),
	"tofu":      flagSet("-chdir"),
	"npm":       flagSet("--prefix", "-w", "--workspace", "--registry", "--cache", "--userconfig"),
	"pnpm":      flagSet("-C", "--dir", "--filter", "-F", "-w", "--workspace"),
	"yarn":      flagSet("--cwd"),
	"cargo":     flagSet("--manifest-path", "-Z", "--config", "--color", "-C"),
	"go":        flagSet("-C"),
	"pip":       flagSet("--python", "--log", "--proxy", "--cache-dir", "--retries", "--timeout"),
	"pip3":      flagSet("--python", "--log", "--proxy", "--cache-dir", "--retries", "--timeout"),
	"uv":        flagSet("--directory", "--project", "--cache-dir", "--config-file", "--color"),
	"poetry":    flagSet("-C", "--directory", "-P", "--project"),
	"dotnet":    flagSet(),
	"brew":      flagSet(),
}

// positionals returns the call's arguments that are not options, skipping the values of the tool's global options and stopping at "--".
func positionals(call shellCall) []string {
	values := toolValueOptions[call.Name]
	var out []string
	for i := 0; i < len(call.Args); i++ {
		arg := call.Args[i]
		if arg == "--" {
			break
		}
		if strings.HasPrefix(arg, "-") && len(arg) > 1 {
			if values[arg] {
				i++
			}
			continue
		}
		out = append(out, arg)
	}
	return out
}

// subMatch returns the subcommand path that leads the call's positionals, or "" when none does.
func subMatch(subs []string, words []string) string {
	for _, sub := range subs {
		parts := strings.Fields(sub)
		if len(parts) > len(words) {
			continue
		}
		ok := true
		for i, part := range parts {
			if part != "*" && part != words[i] {
				ok = false
				break
			}
		}
		if ok {
			return sub
		}
	}
	return ""
}

// flagMatch returns the first of flags the arguments carry, or "".
func flagMatch(flags []string, args []string) string {
	for _, flag := range flags {
		if strings.HasPrefix(flag, "--") {
			if hasLongFlag(args, flag) {
				return flag
			}
			continue
		}
		if strings.HasPrefix(flag, "-") && len(flag) == 2 && hasShortFlag(args, flag[1:]) {
			return flag
		}
	}
	return ""
}

func argMatch(args, literals, prefixes []string) string {
	for _, arg := range args {
		for _, literal := range literals {
			if arg == literal {
				return arg
			}
		}
		for _, prefix := range prefixes {
			if strings.HasPrefix(arg, prefix) {
				return arg
			}
		}
	}
	return ""
}

func unlessMatch(unless []string, args []string) bool {
	for _, exempt := range unless {
		for i, arg := range args {
			if arg == exempt {
				return true
			}
			if name, value, ok := strings.Cut(exempt, "="); ok && arg == name && i+1 < len(args) && args[i+1] == value {
				return true
			}
		}
	}
	return false
}

func (f form) toolMatches(name string) bool {
	if len(f.Tools) == 0 && len(f.ToolPrefixes) == 0 {
		return true
	}
	for _, tool := range f.Tools {
		if tool == name {
			return true
		}
	}
	for _, prefix := range f.ToolPrefixes {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// match reports whether the form matches the call and says what matched.
func (f form) match(call shellCall, words []string, env matchEnv) (string, bool) {
	if !f.toolMatches(call.Name) {
		return "", false
	}
	shown := call.Name
	if len(f.Subs) > 0 {
		sub := subMatch(f.Subs, words)
		if sub == "" {
			return "", false
		}
		shown += " " + strings.Join(words[:len(strings.Fields(sub))], " ")
	}
	if len(f.NoSubs) > 0 && subMatch(f.NoSubs, words) != "" {
		return "", false
	}
	var with []string
	if len(f.Flags) > 0 {
		flag := flagMatch(f.Flags, call.Args)
		if flag == "" {
			return "", false
		}
		with = append(with, flag)
	}
	if len(f.Args) > 0 || len(f.ArgPrefixes) > 0 {
		arg := argMatch(call.Args, f.Args, f.ArgPrefixes)
		if arg == "" {
			return "", false
		}
		with = append(with, arg)
	}
	if len(f.Unless) > 0 && unlessMatch(f.Unless, call.Args) {
		return "", false
	}
	if f.Match != nil {
		detail, ok := f.Match(call, env)
		if !ok {
			return "", false
		}
		if detail != "" {
			return detail, true
		}
	}
	if len(with) > 0 {
		shown += " with " + strings.Join(with, " ")
	}
	return "Matched: " + shown + ".", true
}

// matchCall returns a verdict for every rule one of whose forms matches the call, in catalog order.
func matchCall(call shellCall, env matchEnv) []Verdict {
	words := positionals(call)
	var found []Verdict
	for i := range catalogRules {
		r := &catalogRules[i]
		for _, f := range r.Forms {
			if detail, ok := f.match(call, words, env); ok {
				found = append(found, Verdict{RuleID: r.ID, Category: r.Category, Description: r.Description, Mode: r.Mode, Detail: detail})
				break
			}
		}
	}
	return found
}

// writePathVerdicts returns a verdict for every rule whose WritePaths cover a path being written.
func writePathVerdicts(raw, resolved, home string) []Verdict {
	var found []Verdict
	for i := range catalogRules {
		r := &catalogRules[i]
		for _, pattern := range r.WritePaths {
			if writePathMatches(pattern, raw, resolved, home) {
				found = append(found, Verdict{RuleID: r.ID, Category: r.Category, Description: r.Description, Mode: r.Mode, Detail: "Matched: a write to " + resolved + "."})
				break
			}
		}
	}
	return found
}

func writePathMatches(pattern, raw, resolved, home string) bool {
	dir := strings.HasSuffix(pattern, "/")
	if strings.HasPrefix(pattern, "~/") {
		if home == "" {
			return false
		}
		target := strings.TrimRight(home, "/") + "/" + strings.TrimPrefix(pattern, "~/")
		if dir {
			return strings.HasPrefix(resolved, target) || resolved == strings.TrimSuffix(target, "/")
		}
		if strings.HasSuffix(target, "*") {
			return strings.HasPrefix(resolved, strings.TrimSuffix(target, "*"))
		}
		return resolved == target
	}
	for _, text := range []string{"/" + strings.TrimPrefix(raw, "./"), resolved} {
		if dir {
			if strings.Contains(text+"/", "/"+pattern) {
				return true
			}
			continue
		}
		if strings.HasSuffix(text, "/"+pattern) {
			return true
		}
	}
	return false
}
