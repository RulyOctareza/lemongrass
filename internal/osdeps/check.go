// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"maps"
	"sync"
)

// Report is the check's output, printed by lgrassd deps check --json. The app builds against this shape.
type Report struct {
	OS   OSInfo      `json:"os"`
	Deps []DepStatus `json:"deps"`
}

// Check finds every dependency and decides what to offer for it. Every binary is resolved again on each run, which
// costs a few stat calls; its version and origin are read again only when the file changed since the cached run, or
// always when fresh is set. A cache that cannot be written leaves the report intact and is returned as the error.
func Check(ctx context.Context, sys System, cachePath string, fresh bool) (Report, error) {
	info := DetectOS(ctx, sys)
	var cached map[string]cacheEntry
	if !fresh {
		cached = loadCache(cachePath, info)
	}
	report := Report{OS: info, Deps: make([]DepStatus, len(Deps))}
	entries := map[string]cacheEntry{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for i, dep := range Deps {
		wg.Go(func() {
			path := Resolve(ctx, sys, dep.Name)
			if path == "" {
				report.Deps[i] = evaluate(dep, info, "", "", Origin{Kind: OriginNone})
				return
			}
			st, _ := sys.Stat(path)
			entry, ok := cached[dep.Name]
			if !ok || entry.Path != path || entry.Size != st.Size || entry.ModTime != st.ModTime {
				out, _ := sys.Run(ctx, path, dep.VersionArgs...)
				entry = cacheEntry{
					Path:    path,
					Size:    st.Size,
					ModTime: st.ModTime,
					Version: ParseVersion(dep.Name, string(out)),
					Origin:  readOrigin(ctx, sys, info, path),
				}
			}
			// A timeout or a held rpm lock must not stick until the binary changes, so an unread origin on a
			// family that has one, or an unread version that a floor depends on, is not cached.
			unsettled := (entry.Origin.Kind == OriginUnknown && info.Family == FamilyFedora) ||
				(entry.Version == "" && dep.Floor != "")
			if !unsettled {
				mu.Lock()
				entries[dep.Name] = entry
				mu.Unlock()
			}
			report.Deps[i] = evaluate(dep, info, path, entry.Version, entry.Origin)
		})
	}
	wg.Wait()
	if cached != nil && maps.Equal(cached, entries) {
		return report, nil
	}
	return report, saveCache(cachePath, info, entries)
}
