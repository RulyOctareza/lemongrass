// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"strings"
)

// Origin kinds: who put the binary there.
const (
	OriginPackage = "package"
	OriginManual  = "manual"
	OriginNone    = "none"
	// A family whose package database the check does not read yet.
	OriginUnknown = "unknown"
)

// Origin is who installed a dependency. Package is set only for OriginPackage.
type Origin struct {
	Kind    string `json:"kind"`
	Package string `json:"package,omitempty"`
}

// readOrigin asks the package database who owns the binary. It reads the real path, because /usr/bin entries can be
// links through /etc/alternatives that no package owns themselves.
func readOrigin(ctx context.Context, sys System, info OSInfo, path string) Origin {
	if path == "" {
		return Origin{Kind: OriginNone}
	}
	if info.Family != FamilyFedora {
		return Origin{Kind: OriginUnknown}
	}
	real, err := sys.EvalSymlinks(path)
	if err != nil {
		real = path
	}
	out, err := sys.Run(ctx, "rpm", "-qf", "--qf", "%{NAME}\n", real)
	if err == nil {
		// A multilib file has one owner per arch, all with the same name.
		if name, _, _ := strings.Cut(strings.TrimSpace(string(out)), "\n"); name != "" {
			return Origin{Kind: OriginPackage, Package: strings.TrimSpace(name)}
		}
		return Origin{Kind: OriginUnknown}
	}
	// rpm exits 1 for a missing file too, so an unowned file is told apart by its text and by the file still being there.
	if _, ok := sys.Stat(real); ok && strings.Contains(string(out), "is not owned by any package") {
		return Origin{Kind: OriginManual}
	}
	return Origin{Kind: OriginUnknown}
}
