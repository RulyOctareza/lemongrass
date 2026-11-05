// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

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
	Category    string
	Description string
	Mode        Mode
	// Detail says what in the command matched the rule, or for a deletion why it was not exempt and what would pass.
	Detail string
}

// A guarded command reaches the human as a command ask, which runs it in a terminal on the decision card, so a refusal points there and never at typing the command into the prompt.
const commandAskLine = " If it is truly needed, file a command ask so the user can run it: run lgrass how ask request."

func (v Verdict) Message() string {
	detail := ""
	if v.Detail != "" {
		detail = " " + v.Detail
	}
	if deletionScopeRules[v.RuleID] && v.Detail != "" {
		subject := "rm"
		if v.RuleID == "find-delete" {
			subject = "find -delete"
		}
		return "lgrass: " + subject + " blocked (rule " + v.RuleID + ")." + detail + commandAskLine
	}
	if v.Mode == ModeAsk {
		return "lgrass: rule " + v.RuleID + " (" + v.Description + ") needs the user's decision." + detail + " A shell command goes to the user as a command ask: run lgrass how ask request. For anything else, tell the user what you need and why."
	}
	return "lgrass: this call is blocked by rule " + v.RuleID + " (" + v.Description + ")." + detail + " It is not allowed in a lemongrass session, whatever the permission mode." + commandAskLine
}

var secretComponents = map[string]bool{".ssh": true, ".aws": true, ".gnupg": true, ".kube": true, ".lemongrass": true, ".netrc": true}

// Compared lowercased in slash spelling on Windows, where the keeper is the scheduled task \Lemongrass\lgrassconf and the app installs per user.
var guardFragments = []string{".claude/settings", "/.local/bin/lgrass", "/usr/local/bin/lgrass", "lgrassconf.service", ".codex/hooks.json", ".codex/config.toml", "/windows/system32/tasks/lemongrass/", "/appdata/local/programs/lemongrass/"}

var accountSettingsPattern = regexp.MustCompile(`\.claude-[a-z0-9-]+/settings`)

var readOnlyCommands = map[string]bool{
	"cat": true, "ls": true, "head": true, "tail": true, "grep": true, "rg": true, "less": true, "more": true,
	"stat": true, "file": true, "wc": true, "diff": true, "jq": true, "realpath": true, "readlink": true,
	"which": true, "test": true, "[": true, "echo": true, "printf": true, "du": true, "md5sum": true, "sha256sum": true,
}

var downloaders = map[string]bool{"curl": true, "wget": true, "iwr": true, "irm": true, "invoke-webrequest": true, "invoke-restmethod": true}

var pipeTargets = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "dash": true, "ksh": true, "python": true, "python3": true,
	"perl": true, "ruby": true, "node": true, "php": true, "deno": true, "bun": true, "source": true,
	"iex": true, "invoke-expression": true, "powershell": true, "pwsh": true, "cmd": true,
}

var deviceRedirectPrefixes = []string{"/dev/sd", "/dev/nvme", "/dev/hd", "/dev/vd", "/dev/xvd", "/dev/mmcblk", "/dev/disk", "/dev/mapper", "/dev/dm-"}

// windowsRawDevices are the device namespace paths of a disk or volume, such as \\.\PhysicalDrive0 or \\.\C:, read in their folded slash spelling. Git Bash's /dev/sdX names the same disks and is in the prefix lists.
var windowsRawDevices = regexp.MustCompile(`^//\./(physicaldrive|harddisk|cdrom|tape)|^//\./[a-z]:`)

// rawDeviceTarget reports a path naming a raw disk, by one of prefixes or as a Windows device path.
func rawDeviceTarget(p string, prefixes []string) bool {
	spelled, _ := spellPath(p, "")
	text := filepath.ToSlash(foldText(spelled))
	for _, prefix := range prefixes {
		if strings.HasPrefix(p, prefix) || strings.HasPrefix(text, prefix) {
			return true
		}
	}
	return windowsRawDevices.MatchString(text)
}

type checker struct {
	in     Input
	policy Policy
	// home is in.Home resolved by ResolveDir, for every ~ expansion and comparison with a resolved path; matchEnv keeps in.Home as given, since its checks compare raw arguments.
	home string
}

func Decide(in Input, policy Policy) *Verdict {
	c := checker{in: in, policy: policy, home: ResolveDir(in.Home)}
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
		return c.pick([]Verdict{ruleVerdict("unparseable-command", "")})
	}
	var found []Verdict
	if c.in.Cwd != "" && hasSecretComponent(c.in.Cwd) {
		found = append(found, secretVerdict())
	}
	return c.pick(append(found, c.scriptVerdicts(script)...))
}

// scriptVerdicts judges one parsed script: every call against the catalog and the engine checks, its redirects and its pipelines. A script read at judge time, such as a shell file run by an interpreter, is judged through it too.
func (c checker) scriptVerdicts(script shellScript) []Verdict {
	var found []Verdict
	env := matchEnv{Home: c.in.Home, Cwd: c.in.Cwd, ProjectRoot: c.in.ProjectRoot}
	dirs, scopeReason := c.deletionDirs(script)
	for _, call := range script.Calls {
		if call.Dynamic {
			found = append(found, ruleVerdict("dynamic-command", ""))
			continue
		}
		for _, verdict := range matchCall(call, env) {
			if deletionScopeRules[verdict.RuleID] {
				verdict.Detail = scopeReason
				if verdict.Detail == "" {
					verdict.Detail = c.deletionCheck(call, dirs)
				}
				if verdict.Detail == "" {
					if c.policy.Rules[verdict.RuleID] != overrideDeny {
						continue
					}
					verdict.Detail = "The user's Safety policy sets " + verdict.RuleID + " to deny."
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
	found = append(found, c.interpreterVerdicts(script)...)
	return found
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
			found = append(found, ruleVerdict("raw-network", "Matched: a redirect to "+redirect.Path+"."))
		}
		for _, candidate := range c.pathCandidates(redirect.Path) {
			found = append(found, c.pathChecks(candidate, redirect.Write)...)
			if !redirect.Write {
				continue
			}
			if rawDeviceTarget(candidate, deviceRedirectPrefixes) {
				found = append(found, ruleVerdict("raw-device-write", "Matched: a redirect to "+candidate+"."))
			}
		}
	}
	return found
}

func (c checker) pathChecks(candidate string, guardApplies bool) []Verdict {
	var found []Verdict
	resolved, kind := resolvePath(candidate, c.in.Cwd, c.home)
	if kind == spellingUnmappable {
		found = append(found, ruleVerdict("unparseable-command", "Matched: '"+candidate+"' is a path form the guard cannot reduce to a drive path."))
	}
	// The raw text is compared in its normalized spelling too, so a Windows spelling of a secret or guard path is seen before it resolves.
	spelled, _ := spellPath(candidate, c.in.Cwd)
	candidate = foldText(spelled)
	if hasSecretComponent(candidate) || hasSecretComponent(resolved) {
		found = append(found, secretVerdict())
	}
	if c.policy.blocksPath(resolved, c.home) {
		found = append(found, Verdict{RuleID: "custom-path", Description: "a path the user blocked", Mode: ModeDeny})
	}
	if guardApplies && mentionsGuard(candidate, resolved) {
		found = append(found, guardVerdict())
	}
	if guardApplies {
		found = append(found, writePathVerdicts(candidate, resolved, c.home)...)
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
		if matches, err := globPath(ResolvePath(candidate, c.in.Cwd, c.home)); err == nil {
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
			case overrideAsk, overrideApproval:
				verdict.Mode = ModeAsk
			}
		}
		if verdict.Mode == ModeAllow {
			continue
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
	verdict := ruleVerdict("download-exec", "Matched: a download piped or substituted into an interpreter.")
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

// OpaqueShellVerdict is the deny for a shell call whose command text is not POSIX shell, such as a PowerShell tool call, which the gate cannot read.
func OpaqueShellVerdict(detail string) *Verdict {
	verdict := ruleVerdict("opaque-shell", detail)
	return &verdict
}

func secretVerdict() Verdict {
	return ruleVerdict("secret-path", "")
}

func guardVerdict() Verdict {
	return ruleVerdict("guard-config", "")
}

func readOnlyCall(call shellCall) bool {
	if readOnlyCommands[call.Name] {
		return true
	}
	return call.Name == "sed" && !hasShortFlag(call.Args, "i") && !hasLongFlag(call.Args, "--in-place")
}

func hasSecretComponent(p string) bool {
	clean := filepath.ToSlash(foldText(p))
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
	for _, text := range []string{filepath.ToSlash(foldText(raw)), filepath.ToSlash(foldText(resolved))} {
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

// ResolvePath expands ~ and $HOME, anchors a relative path at cwd, and resolves symlinks in the parent directory, or in the whole path when it is written with a trailing slash.
func ResolvePath(p, cwd, home string) string {
	resolved, _ := resolvePath(p, cwd, home)
	return resolved
}

// resolvePath is ResolvePath that also says whether the written spelling could be normalized; see spellPath.
func resolvePath(p, cwd, home string) (string, spellingKind) {
	p, kind := spellPath(strings.TrimSpace(p), cwd)
	// A trailing slash makes the OS follow the last component when it is a symlink, so rm -r link/ reaches what link points to.
	follow := strings.HasSuffix(p, "/") && !isRootPath(filepath.Clean(p))
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
	// Symlinks are resolved through the deepest existing ancestor, as ResolveDir resolves the home, so a path into a directory that does not exist yet is spelled the same way as the home it is compared with. Cleaning first drops a trailing slash, which would otherwise make the path its own parent and double its last component.
	p = filepath.Clean(p)
	if follow {
		return resolveFully(p), kind
	}
	return locate(p), kind
}

// locate resolves symlinks in p's parent and keeps its last component as written, the location the OS acts on for a path without a trailing slash.
func locate(p string) string {
	return foldPath(filepath.Join(resolveExisting(filepath.Dir(p)), filepath.Base(p)))
}

// writtenAbs reports whether a path is absolute as written, in its normalized spelling.
func writtenAbs(p string) bool {
	spelled, _ := spellPath(p, "")
	return filepath.IsAbs(spelled)
}

// isRootPath reports the root of the filesystem or of a Windows volume.
func isRootPath(p string) bool {
	if p == "/" {
		return true
	}
	volume := filepath.VolumeName(p)
	return volume != "" && (p == volume+`\` || p == volume+"/")
}
