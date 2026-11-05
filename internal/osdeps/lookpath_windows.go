// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build windows

package osdeps

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// lookPath searches the PATH as the registry holds it now, then the process's own. A running process never sees PATH
// changes, and winget and installers write only the registry, so a tool installed while the app runs would otherwise
// stay missing until the app restarts. Machine entries come before user entries, as Windows orders them.
func lookPath(name string) (string, error) {
	seen := map[string]bool{}
	for _, dir := range append(registryPath(), filepath.SplitList(os.Getenv("PATH"))...) {
		dir = strings.Trim(dir, `"`)
		key := strings.ToLower(filepath.Clean(dir))
		// A relative entry would resolve against the current directory, which exec.LookPath refuses too.
		if dir == "" || !filepath.IsAbs(dir) || seen[key] {
			continue
		}
		seen[key] = true
		// LookPath with a separator in the name tries PATHEXT itself and accepts winget's app execution aliases.
		if p, err := exec.LookPath(filepath.Join(dir, name)); err == nil {
			return p, nil
		}
	}
	return "", &exec.Error{Name: name, Err: errors.New("executable file not found in %PATH%")}
}

func registryPath() []string {
	var dirs []string
	for _, k := range []struct {
		root registry.Key
		path string
	}{
		{registry.LOCAL_MACHINE, `SYSTEM\CurrentControlSet\Control\Session Manager\Environment`},
		{registry.CURRENT_USER, `Environment`},
	} {
		key, err := registry.OpenKey(k.root, k.path, registry.QUERY_VALUE)
		if err != nil {
			continue
		}
		value, kind, err := key.GetStringValue("Path")
		key.Close()
		if err != nil {
			continue
		}
		if kind == registry.EXPAND_SZ {
			if expanded, err := registry.ExpandString(value); err == nil {
				value = expanded
			}
		}
		dirs = append(dirs, filepath.SplitList(value)...)
	}
	return dirs
}
