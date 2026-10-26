// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

export const ACCOUNT_NAME = /^[a-z0-9-]{1,32}$/

// One entry per thinker, '' for primary; undefined means no choice was made and null means the list is unusable.
export function cleanThinkerAccounts(
  value: unknown,
  memberCount: number
): string[] | null | undefined {
  if (value === undefined) return undefined
  if (!Array.isArray(value) || value.length !== memberCount) return null
  for (const entry of value) {
    if (typeof entry !== 'string') return null
    if (entry !== '' && !ACCOUNT_NAME.test(entry)) return null
  }
  return value as string[]
}
