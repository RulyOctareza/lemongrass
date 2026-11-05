// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

//go:build !windows

package guard

import (
	"path/filepath"
	"strings"
)

// Off Windows every spelling is already the form the rules compare, so each hook is identity and Linux and macOS verdicts are what they were before the Windows layer existed. normalize_windows.go holds the same functions for Windows.

func spellPath(p, _ string) (string, spellingKind) { return p, spellingOK }

func foldPath(p string) string { return p }

func foldText(s string) string { return s }

func commandBase(name string) string { return filepath.Base(name) }

func namesPath(name string) bool { return strings.Contains(name, "/") }

func unescapeDoubleQuoted(value string) string { return unescapeLiteral(value) }

func globPath(pattern string) ([]string, error) { return filepath.Glob(pattern) }

func pathLiteral(string) bool { return true }

func platformSystemDirs() []string { return nil }
