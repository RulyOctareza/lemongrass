// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package config

import "path/filepath"

// Exe is the file name of one of lemongrass's own binaries (lgrass, lgrassd, lgrassconf) on this OS.
func Exe(name string) string {
	return name + exeSuffix
}

// LgrassdPath is where the app installs lgrassd. It is off PATH, so a model cannot call it by name, and the keeper
// registers hooks only when it exists.
func LgrassdPath(home string) string {
	return filepath.Join(home, ".lemongrass", "bin", Exe("lgrassd"))
}

// PathBinDir is where the app installs lgrass and lgrassconf: ~/.local/bin on every OS. On Windows nothing puts it on
// PATH by default, so `lgrassconf install` adds it to the user's PATH.
func PathBinDir(home string) string {
	return filepath.Join(home, ".local", "bin")
}

// BinDirs are every folder the app may have copied lgrass and lgrassconf into, PathBinDir first. It must match
// resolveInstallDir in ui/src/main/lgrassInstall.ts.
func BinDirs(home string) []string {
	return append([]string{PathBinDir(home)}, fallbackBinDirs...)
}
