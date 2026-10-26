// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package gatekeeper

import "testing"

func TestNewBackendAttachesAuditStoreToBothGates(t *testing.T) {
	b, err := NewBackend(t.TempDir())
	if err != nil {
		t.Fatalf("NewBackend: %v", err)
	}
	if b.DB.Audit == nil {
		t.Error("DB.Audit is nil, want the shared audit store")
	}
	if b.HTTP.Audit == nil {
		t.Error("HTTP.Audit is nil, want the shared audit store")
	}
	if b.DB.Audit != b.HTTP.Audit {
		t.Error("DB.Audit and HTTP.Audit are different stores, want the same one")
	}
}
