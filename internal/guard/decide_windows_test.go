// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

// gitBashSpelling writes a Windows path the way Git Bash does, C:\a\b as /c/a/b.
func gitBashSpelling(p string) string {
	volume := filepath.VolumeName(p)
	return "/" + strings.ToLower(volume[:1]) + filepath.ToSlash(p[len(volume):])
}

func quoted(p string) string { return "'" + p + "'" }

func ruleIn(t *testing.T, in Input, policy Policy) string {
	t.Helper()
	return ruleOf(Decide(in, policy))
}

func TestWindowsSecretPathSpellings(t *testing.T) {
	home := resolvedTempDir(t)
	if err := os.MkdirAll(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	spellings := []string{
		`"` + home + `\.ssh\id_rsa"`,
		quoted(home + `\.SSH\id_rsa`),
		quoted(strings.ToUpper(home) + `\.ssh\id_rsa`),
		gitBashSpelling(home) + "/.ssh/id_rsa",
		quoted(`%USERPROFILE%\.ssh\id_rsa`),
		quoted(`$env:USERPROFILE\.ssh\id_rsa`),
		quoted(`~\.ssh\id_rsa`),
		quoted(home + `\.ssh::$DATA`),
		quoted(home + `\.ssh.\id_rsa`),
		quoted(`\\?\` + home + `\.ssh\id_rsa`),
	}
	for _, spelling := range spellings {
		in := Input{Tool: "Bash", Command: "cat " + spelling, Cwd: t.TempDir(), Home: home}
		if got := ruleIn(t, in, Policy{}); got != "secret-path" {
			t.Errorf("cat %s = %q, want secret-path", spelling, got)
		}
	}
	read := Input{Tool: "Read", Paths: []string{home + `\.SSH\id_rsa`}, Cwd: t.TempDir(), Home: home}
	if got := ruleIn(t, read, Policy{}); got != "secret-path" {
		t.Errorf("Read of .SSH = %q, want secret-path", got)
	}
}

func TestWindowsGuardConfigSpellings(t *testing.T) {
	home := resolvedTempDir(t)
	for _, command := range []string{
		"echo x > " + quoted(home+`\.Claude\Settings.json`),
		"cp x " + quoted(home+`\.local\bin\LGRASS.EXE`),
		"cp x " + gitBashSpelling(home) + "/.local/bin/lgrass.exe",
		"cp x " + quoted(`C:\Windows\System32\Tasks\Lemongrass\lgrassconf`),
		"cp x " + quoted(home+`\AppData\Local\Programs\Lemongrass\lemongrass.exe`),
	} {
		in := Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: home}
		if got := ruleIn(t, in, Policy{}); got != "guard-config" {
			t.Errorf("%s = %q, want guard-config", command, got)
		}
	}
}

func TestWindowsUnmappablePathIsDenied(t *testing.T) {
	for _, path := range []string{`\\?\Volume{1b3b1146-4076-11e1-84aa-806e6f6e6963}\x`, `\\?\GLOBALROOT\Device\HarddiskVolume1\x`, `\??\C:\x`} {
		in := Input{Tool: "Bash", Command: "cat " + quoted(path), Cwd: t.TempDir(), Home: t.TempDir()}
		if got := ruleIn(t, in, Policy{}); got != "unparseable-command" {
			t.Errorf("cat %s = %q, want unparseable-command", path, got)
		}
	}
}

func windowsProject(t *testing.T) (root, home, outside string) {
	t.Helper()
	home = resolvedTempDir(t)
	root = filepath.Join(home, "work", "app")
	outside = resolvedTempDir(t)
	for _, dir := range []string{"build", ".git", `biblio\books`, `biblio\scratchpad\task`} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root, home, outside
}

func TestWindowsDeletionScope(t *testing.T) {
	root, home, outside := windowsProject(t)
	run := func(command, cwd string) string {
		return ruleIn(t, Input{Tool: "Bash", Command: command, Cwd: cwd, Home: home, ProjectRoot: root}, Policy{})
	}
	for _, command := range []string{
		"rm -rf build",
		"RM.EXE -rf build",
		"rm -rf " + quoted(strings.ToUpper(root)+`\build`),
		"rm -rf " + gitBashSpelling(root) + "/build",
		"rm -rf " + quoted(`biblio\scratchpad\task`),
		"rm -rf " + quoted(root+`\build\`),
	} {
		if got := run(command, root); got != "" {
			t.Errorf("%s inside the project = %q, want it to pass", command, got)
		}
	}
	if got := run("rm -rf build", gitBashSpelling(root)); got != "" {
		t.Errorf("rm -rf build from a Git Bash cwd = %q, want it to pass", got)
	}
	if got := run("rm -rf build", strings.ToUpper(root)); got != "" {
		t.Errorf("rm -rf build from an uppercase cwd = %q, want it to pass", got)
	}
	for _, command := range []string{
		"rm -rf .GIT",
		"rm -rf " + quoted(`.git::$DATA`),
		"rm -rf " + quoted(`.git.`),
		"rm -rf " + quoted(`BIBLIO\books`),
		"rm -rf " + quoted(outside),
		"rm -rf " + gitBashSpelling(outside),
		"rm -rf " + quoted(`\Windows\Temp\x`),
		"rm -rf " + quoted(`%TEMP%\x`),
		"rm -rf " + quoted(`C:\NOSUCH~1\x`),
		"Rm.exe -rf " + quoted(outside),
	} {
		if got := run(command, root); got != "rm-force" && got != "dynamic-target" {
			t.Errorf("%s = %q, want rm-force or dynamic-target", command, got)
		}
	}
}

func TestWindowsCommandNames(t *testing.T) {
	cases := map[string]string{
		"GIT.EXE push --force":                                  "git-push-force",
		quoted(`C:\Program Files\Git\cmd\git.exe`) + " push -f": "git-push-force",
		"/c/Program\\ Files/Git/cmd/git.exe reset --hard":       "git-reset-hard",
		"Taskkill.exe /F /IM node.exe":                          "kill-by-name",
		"SCHTASKS /Create /TN x /XML t.xml":                     "scheduled-task",
		"REG.exe ADD HKCU/Software/x":                           "registry-write",
		"CMD.EXE /c dir":                                        "opaque-shell",
		"PowerShell.exe -NoProfile -Command Get-Date":           "opaque-shell",
		"Remove-Item -Recurse -Force x":                         "windows-delete",
		"Stop-Computer":                                         "power",
		"DiskPart":                                              "disk-tool",
	}
	for command, want := range cases {
		in := Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: t.TempDir()}
		if got := ruleIn(t, in, Policy{}); got != want {
			t.Errorf("%s = %q, want %q", command, got, want)
		}
	}
}

func TestWindowsRawDevices(t *testing.T) {
	for _, command := range []string{"echo x > " + quoted(`\\.\PhysicalDrive0`), "echo x > " + quoted(`\\.\C:`), "echo x > /dev/sda"} {
		in := Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: t.TempDir()}
		if got := ruleIn(t, in, Policy{}); got != "raw-device-write" {
			t.Errorf("%s = %q, want raw-device-write", command, got)
		}
		if _, refused := NeverExecute(command, t.TempDir(), t.TempDir()); !refused {
			t.Errorf("%s is not on the never-execute list", command)
		}
	}
	for _, command := range []string{"echo x > /dev/null", "echo x 2> nul"} {
		in := Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: t.TempDir()}
		if got := ruleIn(t, in, Policy{}); got != "" {
			t.Errorf("%s = %q, want it to pass", command, got)
		}
	}
}

func TestWindowsNeverExecute(t *testing.T) {
	home := resolvedTempDir(t)
	for _, command := range []string{"rm -rf /c/", "rm -rf " + quoted(`C:\`), "rm -rf " + quoted(os.Getenv("SystemRoot")), "rm -rf " + quoted(`%USERPROFILE%`), "rm -rf " + quoted(home), "rm -rf " + gitBashSpelling(home)} {
		if _, refused := NeverExecute(command, t.TempDir(), home); !refused {
			t.Errorf("%s is not on the never-execute list", command)
		}
	}
	if reason, refused := NeverExecute("rm -rf build", t.TempDir(), home); refused {
		t.Errorf("rm -rf build refused: %s", reason)
	}
}

func TestWindowsPolicyPaths(t *testing.T) {
	home := resolvedTempDir(t)
	outside := resolvedTempDir(t)
	policy := Policy{Paths: []string{`~\Private`, strings.ToUpper(outside) + `\*`}}
	if err := policy.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"cat " + quoted(home+`\private\x`), "cat " + gitBashSpelling(home) + "/PRIVATE", "cat " + gitBashSpelling(outside) + "/a"} {
		in := Input{Tool: "Bash", Command: command, Cwd: t.TempDir(), Home: home}
		if got := ruleIn(t, in, policy); got != "custom-path" {
			t.Errorf("%s = %q, want custom-path", command, got)
		}
	}
	for _, bad := range []string{`%TEMP%\x`, `relative\x`} {
		if err := (Policy{Paths: []string{bad}}).Validate(); err == nil {
			t.Errorf("%q passed validation", bad)
		}
	}
	for _, good := range []string{`C:\secret`, `d:/data/*`, `~\x`, `%USERPROFILE%\x`, `/c/secret`} {
		if err := (Policy{Paths: []string{good}}).Validate(); err != nil {
			t.Errorf("%q refused: %v", good, err)
		}
	}
}

func TestWindowsShortNames(t *testing.T) {
	home := resolvedTempDir(t)
	long := filepath.Join(home, "averylongdirectoryname")
	if err := os.MkdirAll(long, 0o755); err != nil {
		t.Fatal(err)
	}
	short := shortPathName(t, long)
	if strings.EqualFold(short, long) {
		t.Skip("8.3 names are off on this volume")
	}
	if got, want := ResolvePath(short, "", home), ResolvePath(long, "", home); got != want {
		t.Errorf("ResolvePath(%s) = %s, want %s", short, got, want)
	}
	if !pathLiteral(short) {
		t.Errorf("%s exists and should count as literal", short)
	}
	if pathLiteral(filepath.Join(home, "NOSUCH~1")) {
		t.Errorf("a short name that names nothing should not count as literal")
	}
}

func shortPathName(t *testing.T, p string) string {
	t.Helper()
	from, err := syscall.UTF16PtrFromString(p)
	if err != nil {
		t.Fatal(err)
	}
	buf := make([]uint16, syscall.MAX_PATH)
	n, err := syscall.GetShortPathName(from, &buf[0], uint32(len(buf)))
	if err != nil || n == 0 {
		t.Skipf("no short name: %v", err)
	}
	return syscall.UTF16ToString(buf[:n])
}
