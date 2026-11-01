// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import "slices"

// HelperPackages is what lgrassroot may install or upgrade as root, per OS family. It is the only guarantee the
// helper gives: the polkit rule says YES to any process of an admin user, a model tab included, so the worst any
// caller can do is install or upgrade one of these packages from the repositories already enabled. Add a package
// here only if installing or upgrading it as root is harmless.
var HelperPackages = map[string][]string{
	// git-core owns /usr/bin/git, so a below-floor git is upgraded by that name. ffmpeg is RPM Fusion's full build,
	// listed for its upgrade; a fresh install takes ffmpeg-free.
	FamilyFedora: {"git", "git-core", "android-tools", "ffmpeg-free", "ffmpeg"},
}

// HelperAllows reports whether lgrassroot may act on the package on this family.
func HelperAllows(family, pkg string) bool {
	return slices.Contains(HelperPackages[family], pkg)
}
