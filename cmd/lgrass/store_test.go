// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package main

import (
	"testing"

	"github.com/faizalv/lemongrass/internal/session"
	"github.com/faizalv/lemongrass/internal/testenv"
)

const testProjectID = "cmdtestproj"

func openTestStore(t *testing.T) *session.Store {
	t.Helper()
	testenv.Home(t)
	store, err := session.Open(session.DBPath(), testProjectID)
	if err != nil {
		t.Fatalf("session.Open: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}
