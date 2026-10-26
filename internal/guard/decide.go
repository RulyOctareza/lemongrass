// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package guard

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Input is one agent tool call, reduced to what the guard inspects.
type Input struct {
	Tool    string
	Command string
	Paths   []string
	Writes  bool
	Cwd     string
	Home    string
	// ProjectRoot is the project the agent was started in; rm and find -delete inside it are not gated.
	ProjectRoot string
}

type Verdict struct {
	RuleID      string
	Description string
	Mode        Mode
	// Detail says why a deletion was not exempt and what would pass.
	Detail string
}

func (v Verdict) Message() string {
	if v.Detail != "" {
		subject := "rm"
		if v.RuleID == "find-delete" {
			subject = "find -delete"
		}
		return "lgrass: " + subject + " blocked (rule " + v.RuleID + "). " + v.Detail + " If the user really wants it, ask them to run it themselves with the ! prefix in the prompt."
	}
	if v.Mode == ModeApproval {
		return "lgrass: rule " + v.RuleID + " (" + v.Description + ") needs the user's approval, and approval is not available yet. Ask the user to run it themselves with the ! prefix in the prompt."
	}
	return "lgrass: this call is blocked by rule " + v.RuleID + " (" + v.Description + "). It is not allowed in a lemongrass session, whatever the permission mode. If the user really wants it, ask them to run it themselves with the ! prefix in the prompt."
}

var secretComponents = map[string]bool{".ssh": true, ".aws": true, ".gnupg": true, ".kube": true, ".lemongrass": true, ".netrc": true}

var guardFragments = []string{".claude/settings", "/.local/bin/lgrass", "/usr/local/bin/lgrass", "lgrassconf.service", ".codex/hooks.json", ".codex/config.toml"}

var accountSettingsPattern = regexp.MustCompile(`\.claude-[a-z0-9-]+/settings`)

var readOnlyCommands = map[string]bool{
	"cat": true, "ls": true, "head": true, "tail": true, "grep": true, "rg": true, "less": true, "more": true,
	"stat": true, "file": true, "wc": true, "diff": true, "jq": true, "realpath": true, "readlink": true,
	"which": true, "test": true, "[": true, "echo": true, "printf": true, "du": true, "md5sum": true, "sha256sum": true,
}

var downloaders = map[string]bool{"curl": true, "wget": true}

var pipeTargets = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "python": true, "python3": true,
	"perl": true, "ruby": true, "node": true, "php": true, "deno": true, "bun": true, "source": true,
}

var deviceRedirectPrefixes = []string{"/dev/sd", "/dev/nvme", "/dev/hd", "/dev/vd", "/dev/xvd", "/dev/mmcblk", "/dev/disk", "/dev/mapper", "/dev/dm-"}

type checker struct {
	in     Input
	policy Policy
}

func Decide(in Input, policy Policy) *Verdict {
	c := checker{in: in, policy: policy}
	if in.Tool == "Bash" {
		return c.bash()
	}
	return c.toolPaths()
}

func (c checker) bash() *Verdict {
	if strings.TrimSpace(c.in.Command) == "" {
		return nil
	}
	script, err := parseShell(c.in.Command)
	if err != nil {
		return c.pick([]Verdict{{RuleID: "unparseable-command", Description: "the shell command could not be parsed, so it cannot be checked", Mode: ModeDeny}})
	}
	var found []Verdict
	if c.in.Cwd != "" && hasSecretComponent(c.in.Cwd) {
		found = append(found, secretVerdict())
	}
	catalog := dangerCatalog(c.in.Home)
	dirs, scopeReason := c.deletionDirs(script)
	for _, call := range script.Calls {
		if call.Dynamic {
			found = append(found, Verdict{RuleID: "dynamic-command", Description: "the command name is computed at runtime, so it cannot be checked", Mode: ModeDeny})
			continue
		}
		for _, rule := range catalog {
			if !rule.Match(call) {
				continue
			}
			verdict := Verdict{RuleID: rule.ID, Description: rule.Description, Mode: rule.Mode}
			if deletionScopeRules[rule.ID] {
				verdict.Detail = scopeReason
				if verdict.Detail == "" {
					verdict.Detail = c.deletionCheck(call, dirs)
				}
				if verdict.Detail == "" {
					if c.policy.Rules[rule.ID] != overrideDeny {
						continue
					}
					verdict.Detail = "The user's Safety policy sets " + rule.ID + " to deny."
				}
			}
			found = append(found, verdict)
		}
		if c.policy.blocksBinary(call.Name) {
			found = append(found, Verdict{RuleID: "custom-binary", Description: "a binary the user blocked", Mode: ModeDeny})
		}
		found = append(found, c.pathChecks(call.Path, false)...)
		found = append(found, c.callPathVerdicts(call)...)
	}
	found = append(found, c.redirectVerdicts(script)...)
	found = append(found, downloadExecVerdicts(script)...)
	return c.pick(found)
}

func (c checker) toolPaths() *Verdict {
	var found []Verdict
	for _, p := range c.in.Paths {
		for _, candidate := range c.pathCandidates(p) {
			found = append(found, c.pathChecks(candidate, c.in.Writes)...)
		}
	}
	return c.pick(found)
}

func (c checker) callPathVerdicts(call shellCall) []Verdict {
	var found []Verdict
	exempt := patternArgIndexes(call)
	guardApplies := !readOnlyCall(call)
	for i, arg := range call.Args {
		if exempt[i] {
			continue
		}
		for _, candidate := range c.pathCandidates(arg) {
			found = append(found, c.pathChecks(candidate, guardApplies)...)
		}
	}
	return found
}

func (c checker) redirectVerdicts(script shellScript) []Verdict {
	var found []Verdict
	for _, redirect := range script.Redirects {
		if strings.HasPrefix(redirect.Path, "/dev/tcp/") || strings.HasPrefix(redirect.Path, "/dev/udp/") {
			found = append(found, Verdict{RuleID: "raw-network", Description: "opening a raw network connection", Mode: ModeDeny})
		}
		for _, candidate := range c.pathCandidates(redirect.Path) {
			found = append(found, c.pathChecks(candidate, redirect.Write)...)
			if !redirect.Write {
				continue
			}
			for _, prefix := range deviceRedirectPrefixes {
				if strings.HasPrefix(candidate, prefix) {
					found = append(found, Verdict{RuleID: "raw-device-write", Description: "writing to a raw disk device", Mode: ModeDeny})
				}
			}
		}
	}
	return found
}

func (c checker) pathChecks(candidate string, guardApplies bool) []Verdict {
	var found []Verdict
	resolved := ResolvePath(candidate, c.in.Cwd, c.in.Home)
	if hasSecretComponent(candidate) || hasSecretComponent(resolved) {
		found = append(found, secretVerdict())
	}
	if c.policy.blocksPath(resolved, c.in.Home) {
		found = append(found, Verdict{RuleID: "custom-path", Description: "a path the user blocked", Mode: ModeDeny})
	}
	if guardApplies && mentionsGuard(candidate, resolved) {
		found = append(found, guardVerdict())
	}
	return found
}

func (c checker) pathCandidates(arg string) []string {
	candidates := []string{arg}
	if i := strings.Index(arg, "="); i >= 0 && i+1 < len(arg) {
		candidates = append(candidates, arg[i+1:])
	}
	var expanded []string
	for _, candidate := range candidates {
		if !strings.ContainsAny(candidate, "*?[") {
			continue
		}
		if matches, err := filepath.Glob(ResolvePath(candidate, c.in.Cwd, c.in.Home)); err == nil {
			expanded = append(expanded, matches...)
		}
	}
	return append(candidates, expanded...)
}

func (c checker) pick(found []Verdict) *Verdict {
	var approval *Verdict
	for _, verdict := range found {
		if !isLocked(verdict.RuleID) {
			switch c.policy.Rules[verdict.RuleID] {
			case overrideAllow:
				continue
			case overrideDeny:
				verdict.Mode = ModeDeny
			case overrideApproval:
				verdict.Mode = ModeApproval
			}
		}
		if verdict.Mode == ModeDeny {
			picked := verdict
			return &picked
		}
		if approval == nil {
			picked := verdict
			approval = &picked
		}
	}
	return approval
}

func patternArgIndexes(call shellCall) map[int]bool {
	switch call.Name {
	case "grep", "egrep", "fgrep", "rg", "ag", "ack":
	default:
		return nil
	}
	valueFlags := map[string]bool{"-A": true, "-B": true, "-C": true, "-m": true, "-g": true, "-t": true, "-T": true, "-d": true, "--include": true, "--exclude": true, "--type": true, "--glob": true}
	exempt := map[int]bool{}
	hasExplicit := false
	for i, arg := range call.Args {
		if (arg == "-e" || arg == "--regexp") && i+1 < len(call.Args) {
			hasExplicit = true
			exempt[i+1] = true
		}
	}
	if hasExplicit {
		return exempt
	}
	for i := 0; i < len(call.Args); i++ {
		arg := call.Args[i]
		if valueFlags[arg] {
			i++
			continue
		}
		if strings.HasPrefix(arg, "-") {
			continue
		}
		exempt[i] = true
		break
	}
	return exempt
}

func downloadExecVerdicts(script shellScript) []Verdict {
	verdict := Verdict{RuleID: "download-exec", Description: "running downloaded code", Mode: ModeDeny}
	for _, names := range script.Pipelines {
		seenDownloader := false
		for _, name := range names {
			if downloaders[name] {
				seenDownloader = true
			}
			if seenDownloader && pipeTargets[name] {
				return []Verdict{verdict}
			}
		}
	}
	hasDownloader := false
	for _, call := range script.Calls {
		if downloaders[call.Name] {
			hasDownloader = true
		}
	}
	if !hasDownloader {
		return nil
	}
	for _, call := range script.Calls {
		if !pipeTargets[call.Name] {
			continue
		}
		for _, arg := range call.Args {
			if strings.Contains(arg, "$(dynamic)") {
				return []Verdict{verdict}
			}
		}
	}
	return nil
}

func secretVerdict() Verdict {
	return Verdict{RuleID: "secret-path", Description: "access to credentials or lemongrass private state", Mode: ModeDeny}
}

func guardVerdict() Verdict {
	return Verdict{RuleID: "guard-config", Description: "changing the agent settings, hooks or the lgrass binaries", Mode: ModeDeny}
}

func readOnlyCall(call shellCall) bool {
	if readOnlyCommands[call.Name] {
		return true
	}
	return call.Name == "sed" && !hasShortFlag(call.Args, "i") && !hasLongFlag(call.Args, "--in-place")
}

func hasSecretComponent(p string) bool {
	clean := filepath.ToSlash(p)
	if strings.Contains(clean, ".config/gcloud") {
		return true
	}
	for _, part := range strings.Split(clean, "/") {
		if secretComponents[part] {
			return true
		}
	}
	return false
}

func mentionsGuard(raw, resolved string) bool {
	for _, text := range []string{filepath.ToSlash(raw), filepath.ToSlash(resolved)} {
		for _, fragment := range guardFragments {
			if strings.Contains(text, fragment) {
				return true
			}
		}
		if accountSettingsPattern.MatchString(text) {
			return true
		}
	}
	return false
}

// ResolvePath expands ~ and $HOME, anchors a relative path at cwd, and resolves symlinks in the parent directory.
func ResolvePath(p, cwd, home string) string {
	p = strings.TrimSpace(p)
	switch {
	case p == "~" || strings.HasPrefix(p, "~/"):
		p = filepath.Join(home, strings.TrimPrefix(p, "~"))
	case strings.HasPrefix(p, "$HOME"):
		p = filepath.Join(home, strings.TrimPrefix(p, "$HOME"))
	case strings.HasPrefix(p, "${HOME}"):
		p = filepath.Join(home, strings.TrimPrefix(p, "${HOME}"))
	case !filepath.IsAbs(p) && cwd != "":
		p = filepath.Join(cwd, p)
	}
	if resolved, err := filepath.EvalSymlinks(filepath.Dir(p)); err == nil {
		p = filepath.Join(resolved, filepath.Base(p))
	}
	return filepath.Clean(p)
}
