// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

export const MAX_REASON_CHARS = 500

// A decline reason is human text the leader reads: whitespace is collapsed, the length is capped and an empty result is dropped.
export function cleanReason(value: unknown): string | undefined {
  if (typeof value !== 'string') return undefined
  const text = value.replace(/\s+/g, ' ').trim().slice(0, MAX_REASON_CHARS)
  return text || undefined
}
