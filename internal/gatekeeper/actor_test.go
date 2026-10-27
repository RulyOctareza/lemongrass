// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package gatekeeper

import (
	"encoding/json"
	"testing"

	"github.com/faizalv/lemongrass/internal/vault"
)

func TestRequestHTTPPayloadCarriesActor(t *testing.T) {
	b, err := json.Marshal(requestHTTPPayload{ID: vault.ChannelID("real1"), Method: "GET", Path: "/x", Actor: "tab-1"})
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	var got requestHTTPPayload
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	if got.Actor != "tab-1" {
		t.Errorf("Actor = %q, want %q", got.Actor, "tab-1")
	}
}
