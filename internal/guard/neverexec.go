// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"path/filepath"
	"regexp"
	"strings"
)

// The never-execute list is lemongrass's own and sits outside the policy: no user setting relaxes it, and it binds a command the human executes from an ask, where the policy's deny and ask rules do not apply. It covers catastrophic accidents only, so anything a human may reasonably mean to run stays executable.

var systemTopDirs = map[string]bool{
	"/": true, "/bin": true, "/boot": true, "/dev": true, "/etc": true, "/home": true, "/lib": true, "/lib64": true,
	"/opt": true, "/proc": true, "/root": true, "/sbin": true, "/srv": true, "/sys": true, "/usr": true, "/var": true,
}

var privilegeWrappers = map[string]bool{"sudo": true, "doas": true, "pkexec": true, "run0": true}

var diskTools = map[string]bool{"wipefs": true, "fdisk": true, "sfdisk": true, "cfdisk": true, "gdisk": true, "sgdisk": true, "parted": true, "blkdiscard": true}

var containerSockets = []string{"docker.sock", "podman.sock", "containerd.sock", "crio.sock"}

// Kept apart from the catalog's own device list and flag helpers, so a change to the catalog never loosens this list.
var neverExecDevicePrefixes = []string{"/dev/sd", "/dev/nvme", "/dev/hd", "/dev/vd", "/dev/xvd", "/dev/mmcblk", "/dev/disk", "/dev/mapper", "/dev/dm-", "/dev/md", "/dev/loop"}

var functionDef = regexp.MustCompile(`([A-Za-z_:.][A-Za-z0-9_:.]*)\s*\(\)\s*\{([^}]*)\}`)

// isForkBomb finds a function whose body pipes the function into itself in the background, the shape of :(){ :|:& };:.
func isForkBomb(command string) bool {
	for _, m := range functionDef.FindAllStringSubmatch(command, -1) {
		body := strings.Join(strings.Fields(m[2]), "")
		if strings.Contains(body, m[1]+"|"+m[1]+"&") {
			return true
		}
	}
	return false
}

// NeverExecute reports why command is on the never-execute list, or false when it is not. A command that cannot be parsed, or whose command name is computed at runtime, is refused, since the check cannot see what it would run.
func NeverExecute(command, cwd, home string) (string, bool) {
	if isForkBomb(command) {
		return "it is a fork bomb", true
	}
	script, err := parseShell(command)
	if err != nil {
		return "it cannot be parsed, so lemongrass cannot check what it runs", true
	}
	n := neverExec{cwd: cwd, home: filepath.Clean(home)}
	for _, call := range script.Calls {
		if call.Dynamic {
			return "its command name is computed at runtime, so lemongrass cannot check what it runs", true
		}
		if reason := n.call(call); reason != "" {
			return reason, true
		}
	}
	for _, redirect := range script.Redirects {
		if !redirect.Write {
			continue
		}
		target := ResolvePath(redirect.Path, cwd, home)
		for _, prefix := range neverExecDevicePrefixes {
			if strings.HasPrefix(target, prefix) {
				return "it writes to a raw disk device", true
			}
		}
		if n.insideSSH(target) {
			return "it writes into ~/.ssh", true
		}
	}
	return "", false
}

type neverExec struct {
	cwd  string
	home string
}

func (n neverExec) call(call shellCall) string {
	if privilegeWrappers[call.Name] {
		if inner, ok := privilegedCall(call.Args); ok {
			return n.call(inner)
		}
		return ""
	}
	switch {
	case call.Name == "rm":
		return n.rm(call)
	case call.Name == "mv":
		return n.moveSource(call)
	case call.Name == "shred" || call.Name == "srm" || call.Name == "wipe":
		return n.overwrite(call)
	case call.Name == "chmod" || call.Name == "chown" || call.Name == "chgrp":
		return n.recursivePermissions(call)
	case call.Name == "dd":
		return ddToDevice(call)
	case strings.HasPrefix(call.Name, "mkfs") || call.Name == "mke2fs" || call.Name == "mkswap":
		return "it formats a filesystem"
	case diskTools[call.Name]:
		return "it changes a disk's partitions or signatures"
	case call.Name == "kill" || call.Name == "pkill" || call.Name == "killall":
		return killEverything(call)
	case call.Name == "docker" || call.Name == "podman" || call.Name == "nerdctl":
		return containerRoot(call)
	}
	return ""
}

// privilegedCall drops the wrapper's own flags and returns the command it runs. A flag's value cannot be told from the command without knowing every flag, so it errs towards treating the first bare word as the command.
func privilegedCall(args []string) (shellCall, bool) {
	for i, arg := range args {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		if strings.Contains(arg, "=") && i == 0 {
			continue
		}
		return shellCall{Name: filepath.Base(arg), Path: arg, Args: args[i+1:]}, true
	}
	return shellCall{}, false
}

func (n neverExec) rm(call shellCall) string {
	if neHasLongFlag(call.Args, "--no-preserve-root") {
		return "rm --no-preserve-root removes the whole system"
	}
	recursive := neHasShortFlag(call.Args, "rR") || neHasLongFlag(call.Args, "--recursive")
	for _, target := range operands(call.Args) {
		if reason := n.protectedTarget(target, recursive); reason != "" {
			return "rm " + reason
		}
	}
	return ""
}

func (n neverExec) moveSource(call shellCall) string {
	ops := operands(call.Args)
	if len(ops) < 2 {
		return ""
	}
	for _, source := range ops[:len(ops)-1] {
		if reason := n.protectedTarget(source, true); reason != "" {
			return "mv " + reason
		}
	}
	return ""
}

func (n neverExec) overwrite(call shellCall) string {
	for _, target := range operands(call.Args) {
		resolved := ResolvePath(target, n.cwd, n.home)
		if strings.HasPrefix(resolved, "/dev/") {
			return call.Name + " overwrites a device"
		}
		if n.insideSSH(resolved) {
			return call.Name + " destroys files in ~/.ssh"
		}
	}
	return ""
}

func (n neverExec) recursivePermissions(call shellCall) string {
	if !neHasShortFlag(call.Args, "R") && !neHasLongFlag(call.Args, "--recursive") {
		return ""
	}
	for _, target := range operands(call.Args) {
		resolved := ResolvePath(target, n.cwd, n.home)
		if systemTopDirs[resolved] || resolved == n.home || n.insideSSH(resolved) {
			return call.Name + " -R changes ownership or permissions across " + resolved
		}
	}
	return ""
}

// protectedTarget says why removing or moving target would be catastrophic: the root, a top-level system directory, the home directory, ~/.ssh, a glob over the root, or, for a recursive call, a path whose location is only known at run time.
func (n neverExec) protectedTarget(target string, recursive bool) string {
	if strings.Contains(target, "$") && !strings.HasPrefix(target, "$HOME") && !strings.HasPrefix(target, "${HOME}") {
		if recursive {
			return "of a path built from a variable or substitution could land anywhere, " + target
		}
		return ""
	}
	if strings.HasPrefix(strings.TrimSpace(target), "/*") || strings.TrimSpace(target) == "/." {
		return "targets everything under /"
	}
	resolved := ResolvePath(target, n.cwd, n.home)
	if strings.ContainsAny(target, "*?[") && (resolved == "/" || filepath.Dir(resolved) == "/") {
		return "targets everything under /"
	}
	switch {
	case resolved == "/":
		return "targets /"
	case systemTopDirs[resolved]:
		return "targets the system directory " + resolved
	case n.home != "" && n.home != "." && resolved == n.home:
		return "targets the home directory"
	case n.home != "" && n.home != "." && filepath.Dir(resolved) == n.home && strings.ContainsAny(filepath.Base(target), "*?["):
		return "targets everything in the home directory"
	case n.insideSSH(resolved):
		return "targets ~/.ssh"
	}
	return ""
}

func (n neverExec) insideSSH(resolved string) bool {
	if n.home == "" || n.home == "." {
		return false
	}
	ssh := filepath.Join(n.home, ".ssh")
	return resolved == ssh || strings.HasPrefix(resolved, ssh+"/")
}

func operands(args []string) []string {
	var out []string
	afterDashes := false
	for _, arg := range args {
		if !afterDashes && arg == "--" {
			afterDashes = true
			continue
		}
		if !afterDashes && strings.HasPrefix(arg, "-") && arg != "-" {
			continue
		}
		out = append(out, arg)
	}
	return out
}

func ddToDevice(call shellCall) string {
	for _, arg := range call.Args {
		if !strings.HasPrefix(arg, "of=") {
			continue
		}
		target := strings.TrimPrefix(arg, "of=")
		for _, prefix := range neverExecDevicePrefixes {
			if strings.HasPrefix(target, prefix) {
				return "dd writes to a raw disk device"
			}
		}
	}
	return ""
}

func killEverything(call shellCall) string {
	if call.Name != "kill" {
		if neContainsArg(call.Args, "-1") {
			return call.Name + " -1 kills every process"
		}
		return ""
	}
	for _, arg := range operands(call.Args) {
		if arg == "-1" {
			return "kill -1 kills every process"
		}
	}
	for i, arg := range call.Args {
		if arg == "-1" && i > 0 {
			return "kill -1 kills every process"
		}
	}
	return ""
}

// containerRoot catches the container options that hand the container root on the host: privileged mode, the host's pid namespace, broad capabilities, and a bind of / or a container engine's socket.
func containerRoot(call shellCall) string {
	sub := ""
	rest := call.Args
	for i, arg := range call.Args {
		if !strings.HasPrefix(arg, "-") {
			sub = arg
			rest = call.Args[i+1:]
			break
		}
	}
	if sub == "container" && len(rest) > 0 {
		sub, rest = rest[0], rest[1:]
	}
	if sub != "run" && sub != "create" && sub != "exec" {
		return ""
	}
	for i, arg := range rest {
		value := ""
		if eq := strings.Index(arg, "="); eq >= 0 {
			value = arg[eq+1:]
		} else if i+1 < len(rest) {
			value = rest[i+1]
		}
		name := arg
		if eq := strings.Index(arg, "="); eq >= 0 {
			name = arg[:eq]
		}
		switch name {
		case "--privileged":
			return "a privileged container gains root on the host"
		case "--pid", "--userns", "--ipc":
			if value == "host" {
				return "a container in the host's " + strings.TrimPrefix(name, "--") + " namespace gains root on the host"
			}
		case "--cap-add":
			upper := strings.ToUpper(value)
			if upper == "ALL" || upper == "SYS_ADMIN" || upper == "CAP_SYS_ADMIN" {
				return "a container with " + upper + " gains root on the host"
			}
		case "-v", "--volume", "--mount":
			if hostBind(value) {
				return "a container mounting " + value + " gains root on the host"
			}
		}
		if strings.HasPrefix(arg, "-v") && len(arg) > 2 && hostBind(arg[2:]) {
			return "a container mounting " + arg[2:] + " gains root on the host"
		}
	}
	return ""
}

func hostBind(spec string) bool {
	source := spec
	if strings.Contains(spec, "source=") || strings.Contains(spec, "src=") {
		for _, field := range strings.Split(spec, ",") {
			if v, ok := strings.CutPrefix(field, "source="); ok {
				source = v
			}
			if v, ok := strings.CutPrefix(field, "src="); ok {
				source = v
			}
		}
	} else if colon := strings.Index(spec, ":"); colon >= 0 {
		source = spec[:colon]
	}
	if filepath.Clean(source) == "/" {
		return true
	}
	for _, sock := range containerSockets {
		if strings.HasSuffix(source, sock) {
			return true
		}
	}
	return false
}

func neHasShortFlag(args []string, letters string) bool {
	for _, arg := range args {
		if arg == "--" {
			return false
		}
		if len(arg) > 1 && arg[0] == '-' && arg[1] != '-' && strings.ContainsAny(arg[1:], letters) {
			return true
		}
	}
	return false
}

func neHasLongFlag(args []string, names ...string) bool {
	for _, arg := range args {
		for _, name := range names {
			if arg == name || strings.HasPrefix(arg, name+"=") {
				return true
			}
		}
	}
	return false
}

func neContainsArg(args []string, want string) bool {
	for _, arg := range args {
		if arg == want {
			return true
		}
	}
	return false
}
