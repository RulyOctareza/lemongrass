// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

package vault

import (
	"errors"
	"testing"
	"time"
)

func TestExpireKeylessChannels(t *testing.T) {
	svc := putMultiEnv(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte(unreachableConnString)); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}
	keyless, err := svc.CreateChannel(testRootSecret, "keyless", "app-backend", fullScope(), time.Hour)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	keyed, err := svc.CreateChannel(testRootSecret, "keyed", "app-backend", fullScope(), time.Hour)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	expired, err := svc.CreateChannel(testRootSecret, "expired", "app-backend", fullScope(), -time.Hour)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	httpKeyless, err := svc.CreateHTTPChannel(testRootSecret, "h", "shop", "staging", TypeAPI, HTTPScope{Methods: []string{"GET"}}, nil, time.Hour)
	if err != nil {
		t.Fatalf("CreateHTTPChannel: %v", err)
	}

	// Simulate a vault restart for every channel but keyed.
	svc.mu.Lock()
	delete(svc.activeKeys, keyless.ID)
	delete(svc.activeKeys, expired.ID)
	delete(svc.activeKeys, httpKeyless.ID)
	svc.mu.Unlock()

	now := time.Now()
	if err := svc.ExpireKeylessChannels(now); err != nil {
		t.Fatalf("ExpireKeylessChannels: %v", err)
	}

	if c, _ := svc.ChannelScope(keyless.ID); !c.ExpiresAt.Equal(now) {
		t.Errorf("keyless db channel ExpiresAt = %v, want %v", c.ExpiresAt, now)
	}
	if _, _, err := svc.OpenChannel(keyless.ID); !errors.Is(err, ErrChannelExpired) {
		t.Errorf("OpenChannel on a keyless channel after expiry: got %v, want ErrChannelExpired", err)
	}
	if c, _ := svc.ChannelScope(keyed.ID); !c.ExpiresAt.Equal(keyed.ExpiresAt) {
		t.Errorf("keyed db channel ExpiresAt = %v, want it untouched at %v", c.ExpiresAt, keyed.ExpiresAt)
	}
	if c, _ := svc.ChannelScope(expired.ID); !c.ExpiresAt.Equal(expired.ExpiresAt) {
		t.Errorf("already expired channel ExpiresAt = %v, want it untouched at %v", c.ExpiresAt, expired.ExpiresAt)
	}
	if c, _ := svc.HTTPChannelScope(httpKeyless.ID); !c.ExpiresAt.Equal(now) {
		t.Errorf("keyless HTTP channel ExpiresAt = %v, want %v", c.ExpiresAt, now)
	}
}
