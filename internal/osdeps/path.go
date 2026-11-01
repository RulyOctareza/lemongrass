// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package osdeps

import (
	"context"
	"fmt"
	"sync"
)

// Replaced in tests to stand in for another machine.
var pathSystem = Real

var (
	pathMu sync.Mutex
	found  = map[string]string{}
)

// Path finds a tool for a feature that runs it, without reading its version or origin. A found path is kept while
// the file stays in place; a miss is not kept, so a tool installed while lemongrass runs is picked up on the next call.
func Path(name string) (string, error) {
	pathMu.Lock()
	defer pathMu.Unlock()
	sys := pathSystem()
	if p := found[name]; p != "" {
		if st, ok := sys.Stat(p); ok && !st.Dir {
			return p, nil
		}
		delete(found, name)
	}
	p := Resolve(context.Background(), sys, name)
	if p == "" {
		return "", fmt.Errorf("osdeps: %s is not installed", name)
	}
	found[name] = p
	return p, nil
}
