// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

package agent

import (
	"crypto/rand"
	"fmt"
)

// shortIDAlphabet excludes visually ambiguous characters (0/O, 1/I/L).
const shortIDAlphabet = "23456789ABCDEFGHJKMNPQRSTUVWXYZ"

const shortIDLength = 6

// newShortID generates a six-character model-facing channel id.
func newShortID() (string, error) {
	b := make([]byte, shortIDLength)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("agent: generating channel id: %w", err)
	}
	out := make([]byte, shortIDLength)
	for i, v := range b {
		out[i] = shortIDAlphabet[int(v)%len(shortIDAlphabet)]
	}
	return string(out), nil
}
