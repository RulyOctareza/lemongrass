// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package agent

import "testing"

func TestNewShortIDLengthAndUnique(t *testing.T) {
	a, err := newShortID()
	if err != nil {
		t.Fatalf("newShortID: %v", err)
	}
	b, err := newShortID()
	if err != nil {
		t.Fatalf("newShortID: %v", err)
	}
	if len(a) != shortIDLength {
		t.Errorf("len(newShortID()) = %d, want %d", len(a), shortIDLength)
	}
	if a == b {
		t.Fatalf("two calls to newShortID returned the same id: %q", a)
	}
}

func TestNewShortIDUsesOnlyItsAlphabet(t *testing.T) {
	id, err := newShortID()
	if err != nil {
		t.Fatalf("newShortID: %v", err)
	}
	for _, r := range id {
		found := false
		for _, a := range shortIDAlphabet {
			if r == a {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("newShortID() = %q contains %q, not in shortIDAlphabet", id, r)
		}
	}
}
