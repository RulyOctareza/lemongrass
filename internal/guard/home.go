// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package guard

import "path/filepath"

// ResolveDir resolves a directory that resolved targets are compared with, such as the home, through symlinks the way ResolvePath resolves a target, so both are spelled alike. Without it, a home reached through a symlink (macOS /tmp and /home, Fedora Atomic /home -> /var/home) never matches its resolved targets and loses every home protection. An empty dir stays empty.
func ResolveDir(dir string) string {
	if dir == "" {
		return ""
	}
	return resolveFully(filepath.Clean(dir))
}
