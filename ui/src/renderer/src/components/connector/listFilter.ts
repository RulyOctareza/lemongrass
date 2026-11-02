// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { VaultResterSummary } from '../../../../preload/types'

// True when every space-separated term of query appears, case-insensitively, in one of fields.
// Terms may match different fields, so "staging post" finds a staging channel allowing POST.
export function matchesQuery(query: string, fields: string[]): boolean {
  const terms = query.trim().toLowerCase().split(/\s+/).filter(Boolean)
  if (terms.length === 0) return true
  const hay = fields.map((f) => f.toLowerCase())
  return terms.every((term) => hay.some((f) => f.includes(term)))
}

// The addresses of a rester's environments, or of the one named environment when given.
export function resterDomains(
  summary: VaultResterSummary | undefined,
  environment?: string
): string[] {
  const envs = (summary?.Environments ?? []).filter(
    (e) => environment === undefined || e.Name === environment
  )
  return envs.flatMap((e) => [e.BaseURL, e.Origin]).filter(Boolean)
}
