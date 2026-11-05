// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "testing"

var testWinEnv = winEnv{
	AppData:      `C:\Users\Ann\AppData\Roaming`,
	LocalAppData: `C:\Users\Ann\AppData\Local`,
	CwdDrive:     "C:",
	GitRoot:      `C:\Program Files\Git`,
	TempDir:      `C:\Users\Ann\AppData\Local\Temp`,
}

func TestWindowsSpelling(t *testing.T) {
	cases := []struct {
		in   string
		want string
		kind spellingKind
	}{
		{`C:\Users\Ann\.ssh\id_rsa`, `C:/Users/Ann/.ssh/id_rsa`, spellingOK},
		{`c:/users/ann/.ssh`, `c:/users/ann/.ssh`, spellingOK},
		{`/c/Users/Ann/.ssh`, `C:/Users/Ann/.ssh`, spellingOK},
		{`/C/Users`, `C:/Users`, spellingOK},
		{`/d`, `D:/`, spellingOK},
		{`/cygdrive/d/data`, `D:/data`, spellingOK},
		{`\Windows\System32`, `C:/Windows/System32`, spellingOK},
		{`~\.ssh`, `~/.ssh`, spellingOK},
		{`%USERPROFILE%\.ssh`, `~/.ssh`, spellingOK},
		{`%userprofile%`, `~`, spellingOK},
		{`%HOMEDRIVE%%HOMEPATH%\.aws`, `~/.aws`, spellingOK},
		{`$env:USERPROFILE\.kube`, `~/.kube`, spellingOK},
		{`${env:USERPROFILE}/.kube`, `~/.kube`, spellingOK},
		{`$USERPROFILE/.gnupg`, `~/.gnupg`, spellingOK},
		{`%APPDATA%\npm`, `C:/Users/Ann/AppData/Roaming/npm`, spellingOK},
		{`$env:LOCALAPPDATA\Programs`, `C:/Users/Ann/AppData/Local/Programs`, spellingOK},
		{`%TEMP%\x`, `%TEMP%/x`, spellingVariable},
		{`$env:OTHER\x`, `$env:OTHER/x`, spellingVariable},
		{`\\?\C:\Users\Ann\.ssh`, `C:/Users/Ann/.ssh`, spellingOK},
		{`\\?\UNC\server\share\x`, `//server/share/x`, spellingOK},
		{`\\?\Volume{1b3b1146-4076-11e1-84aa-806e6f6e6963}\x`, `//?/Volume{1b3b1146-4076-11e1-84aa-806e6f6e6963}/x`, spellingUnmappable},
		{`\\?\GLOBALROOT\Device\HarddiskVolume1`, `//?/GLOBALROOT/Device/HarddiskVolume1`, spellingUnmappable},
		{`\??\C:\x`, `/??/C:/x`, spellingUnmappable},
		{`\\.\PhysicalDrive0`, `//./physicaldrive0`, spellingOK},
		{`\\.\C:`, `//./c:`, spellingOK},
		{`\\.\pipe\x`, `//./pipe/x`, spellingUnmappable},
		{`\\server\share\.ssh`, `//server/share/.ssh`, spellingOK},
		{`C:\Users\Ann\.ssh::$DATA`, `C:/Users/Ann/.ssh`, spellingOK},
		{`C:\Users\Ann\.git:hidden\config`, `C:/Users/Ann/.git/config`, spellingOK},
		{`C:\Users\Ann\.ssh.`, `C:/Users/Ann/.ssh`, spellingOK},
		{`C:\Users\Ann\.ssh .\id_rsa`, `C:/Users/Ann/.ssh/id_rsa`, spellingOK},
		{`C:\x\..\y\.`, `C:/x/../y/.`, spellingOK},
		{`C:Usersx`, `Usersx`, spellingOK},
		{`D:data`, `D:/data`, spellingOK},
		{`/etc/hosts`, `C:/Program Files/Git/etc/hosts`, spellingOK},
		{`/`, `C:/Program Files/Git/`, spellingOK},
		{`/tmp/x`, `C:/Users/Ann/AppData/Local/Temp/x`, spellingOK},
		{`/dev/null`, `/dev/null`, spellingOK},
		{`/dev/sda`, `/dev/sda`, spellingOK},
		{`relative\dir\file`, `relative/dir/file`, spellingOK},
		{`~/x`, `~/x`, spellingOK},
		{``, ``, spellingOK},
	}
	for _, tc := range cases {
		got, kind := windowsSpelling(tc.in, testWinEnv)
		if got != tc.want || kind != tc.kind {
			t.Errorf("windowsSpelling(%q) = %q, %d; want %q, %d", tc.in, got, kind, tc.want, tc.kind)
		}
	}
}

func TestWindowsSpellingWithoutGitRoot(t *testing.T) {
	env := testWinEnv
	env.GitRoot = ""
	if got, _ := windowsSpelling("/etc/hosts", env); got != "C:/etc/hosts" {
		t.Errorf("got %q, want the cwd drive's root when the Git root is unknown", got)
	}
}

func TestWindowsCommandName(t *testing.T) {
	cases := map[string]string{
		"rm":                                  "rm",
		"RM.EXE":                              "rm",
		`C:\Program Files\Git\usr\bin\rm.exe`: "rm",
		"/usr/bin/git.exe":                    "git",
		"Git.Exe":                             "git",
		"npm.cmd":                             "npm",
		"install.bat":                         "install",
		"format.com":                          "format",
		"powershell.exe":                      "powershell",
		"Remove-Item":                         "remove-item",
		".exe":                                ".exe",
		"rm.exe.":                             "rm",
		".":                                   ".",
		`C:\tools\script.sh`:                  "script.sh",
	}
	for in, want := range cases {
		if got := windowsCommandName(in); got != want {
			t.Errorf("windowsCommandName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestShortNameSegment(t *testing.T) {
	for _, p := range []string{`C:\PROGRA~1\x`, `C:/Users/RUNNER~1`, `SSH~1`, `c:\docume~1.txt\x`} {
		if !hasShortNameSegment(p) {
			t.Errorf("%q should look like a short name", p)
		}
	}
	for _, p := range []string{`C:\Program Files`, `~/x`, `~`, `backup~`, `a~b`, `toolongname~1`} {
		if hasShortNameSegment(p) {
			t.Errorf("%q should not look like a short name", p)
		}
	}
}

func TestUnescapeDoubleQuotedBash(t *testing.T) {
	cases := map[string]string{
		`C:\Users\x\.ssh`: `C:\Users\x\.ssh`,
		`a\$b`:            `a$b`,
		`a\"b`:            `a"b`,
		`a\\b`:            `a\b`,
		"a\\\nb":          "ab",
		`a\nb`:            `a\nb`,
	}
	for in, want := range cases {
		if got := unescapeDoubleQuotedBash(in); got != want {
			t.Errorf("unescapeDoubleQuotedBash(%q) = %q, want %q", in, got, want)
		}
	}
}
