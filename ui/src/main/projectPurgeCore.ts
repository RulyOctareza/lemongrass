// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

export const PURGE_AFTER_MS = 30 * 24 * 60 * 60 * 1000

// A deletedAt that does not parse is never expired, so a hand-edited registry cannot purge a project early.
export function expiredProjectIds(
  projects: { id: string; deletedAt?: string }[],
  now: number
): string[] {
  return projects
    .filter((p) => {
      if (!p.deletedAt) return false
      const at = Date.parse(p.deletedAt)
      return !Number.isNaN(at) && now - at >= PURGE_AFTER_MS
    })
    .map((p) => p.id)
}
