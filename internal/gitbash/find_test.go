// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gitbash

import (
	"errors"
	"strings"
	"testing"
)

func env(vars map[string]string) func(string) string {
	return func(k string) string { return vars[k] }
}

func files(paths ...string) func(string) bool {
	return func(p string) bool {
		for _, f := range paths {
			if strings.EqualFold(f, p) {
				return true
			}
		}
		return false
	}
}

func noGit(string) (string, error) { return "", errors.New("not found") }

func TestFind(t *testing.T) {
	const stdBash = `C:\Program Files\Git\usr\bin\bash.exe`
	cases := []struct {
		name string
		env  map[string]string
		git  func(string) (string, error)
		have []string
		want string
	}{
		{"claude code variable, bin launcher", map[string]string{"CLAUDE_CODE_GIT_BASH_PATH": `D:\Tools\Git\bin\bash.exe`}, noGit,
			[]string{`D:\Tools\Git\usr\bin\bash.exe`}, `D:\Tools\Git`},
		{"claude code variable, usr bin", map[string]string{"CLAUDE_CODE_GIT_BASH_PATH": `D:\Tools\Git\usr\bin\bash.exe`}, noGit,
			[]string{`D:\Tools\Git\usr\bin\bash.exe`}, `D:\Tools\Git`},
		{"git on PATH under cmd", nil, func(string) (string, error) { return `E:\Git\cmd\git.exe`, nil },
			[]string{`E:\Git\usr\bin\bash.exe`}, `E:\Git`},
		{"git on PATH under mingw64", nil, func(string) (string, error) { return `E:\Git\mingw64\bin\git.exe`, nil },
			[]string{`E:\Git\usr\bin\bash.exe`}, `E:\Git`},
		{"program files default", map[string]string{"ProgramFiles": `C:\Program Files`}, noGit,
			[]string{stdBash}, `C:\Program Files\Git`},
		{"per-user install", map[string]string{"LOCALAPPDATA": `C:\Users\a\AppData\Local`}, noGit,
			[]string{`C:\Users\a\AppData\Local\Programs\Git\usr\bin\bash.exe`}, `C:\Users\a\AppData\Local\Programs\Git`},
		{"WSL bash in System32 is never a root",
			map[string]string{"CLAUDE_CODE_GIT_BASH_PATH": `C:\Windows\System32\bin\bash.exe`, "SystemRoot": `C:\Windows`}, noGit,
			[]string{`C:\Windows\System32\usr\bin\bash.exe`}, ""},
		{"nothing installed", map[string]string{"ProgramFiles": `C:\Program Files`}, noGit, nil, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := find(env(c.env), c.git, files(c.have...))
			if c.want == "" {
				if !errors.Is(err, ErrNotFound) {
					t.Fatalf("got %q, %v; want ErrNotFound", got, err)
				}
				return
			}
			if err != nil || got != c.want {
				t.Fatalf("got %q, %v; want %q", got, err, c.want)
			}
		})
	}
}
