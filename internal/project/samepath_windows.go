// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package project

import "strings"

// samePath compares two resolved paths. Windows names are case-insensitive, and a path EvalSymlinks could not
// resolve keeps the case it was typed in, so the comparison folds case.
func samePath(a, b string) bool {
	return strings.EqualFold(a, b)
}
