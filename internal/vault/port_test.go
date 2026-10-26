// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package vault

import (
	"testing"
	"time"
)

func TestServiceCreateChannelAssignsPortInRange(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	c, err := svc.CreateChannel(testRootSecret, "test-channel", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel: %v", err)
	}
	if c.Port < wireProxyPortRangeStart || c.Port > wireProxyPortRangeEnd {
		t.Errorf("CreateChannel port = %d, want it within [%d, %d]", c.Port, wireProxyPortRangeStart, wireProxyPortRangeEnd)
	}
}

func TestServiceCreateChannelAssignsDistinctPorts(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	a, err := svc.CreateChannel(testRootSecret, "channel-a", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel a: %v", err)
	}
	b, err := svc.CreateChannel(testRootSecret, "channel-b", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel b: %v", err)
	}
	if a.Port == b.Port {
		t.Errorf("CreateChannel assigned the same port %d to both channels", a.Port)
	}
}

func TestServiceCreateChannelReusesRevokedPort(t *testing.T) {
	svc := openTestService(t)
	if err := svc.PutCredential(testRootSecret, "app-backend", []byte("creds")); err != nil {
		t.Fatalf("PutCredential: %v", err)
	}

	a, err := svc.CreateChannel(testRootSecret, "channel-a", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel a: %v", err)
	}
	if err := svc.Revoke(a.ID); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	b, err := svc.CreateChannel(testRootSecret, "channel-b", "app-backend", fullScope(), 5*time.Minute)
	if err != nil {
		t.Fatalf("CreateChannel b: %v", err)
	}
	if b.Port != a.Port {
		t.Errorf("CreateChannel after revoke: port = %d, want the freed port %d reused", b.Port, a.Port)
	}
}
