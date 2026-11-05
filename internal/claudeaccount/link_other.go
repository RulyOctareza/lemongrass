// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package claudeaccount

import "os"

func makeLink(src, dst string) error {
	return os.Symlink(src, dst)
}

func isLink(info os.FileInfo) bool {
	return info.Mode()&os.ModeSymlink != 0
}

func sameLinkTarget(target, src string) bool {
	return target == src
}
