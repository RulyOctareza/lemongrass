// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import "strings"

// Windows paths are handled as strings rather than through path/filepath, which follows the host OS, so the
// Windows probe gives the same answer, and is tested, on Linux. Windows accepts both separators and ignores case.

func winSegments(p string) []string {
	return strings.FieldsFunc(p, func(r rune) bool { return r == '\\' || r == '/' })
}

// winJoin joins with backslashes, dropping empty elements and stray separators at the joins.
func winJoin(elem ...string) string {
	var parts []string
	for _, e := range elem {
		if e = strings.Trim(e, `\/`); e != "" {
			parts = append(parts, e)
		}
	}
	return strings.Join(parts, `\`)
}

func winDir(p string) string {
	if i := strings.LastIndexAny(p, `\/`); i >= 0 {
		return p[:i]
	}
	return ""
}

func winBase(p string) string {
	return p[strings.LastIndexAny(p, `\/`)+1:]
}

// winUnder reports whether p lies inside root and returns the segments below it, in p's own case.
func winUnder(p, root string) ([]string, bool) {
	ps, rs := winSegments(p), winSegments(root)
	if len(rs) == 0 || len(ps) <= len(rs) {
		return nil, false
	}
	for i, r := range rs {
		if !strings.EqualFold(ps[i], r) {
			return nil, false
		}
	}
	return ps[len(rs):], true
}
