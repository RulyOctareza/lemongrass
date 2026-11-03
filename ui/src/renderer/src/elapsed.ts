// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// A running time as a clock reads it: 0:07, 12:34, 1:02:03. A start in the future counts as zero.
export function formatElapsed(ms: number): string {
  const total = Math.max(0, Math.floor(ms / 1000))
  const hours = Math.floor(total / 3600)
  const minutes = Math.floor((total % 3600) / 60)
  const seconds = String(total % 60).padStart(2, '0')
  if (hours === 0) return `${minutes}:${seconds}`
  return `${hours}:${String(minutes).padStart(2, '0')}:${seconds}`
}
