// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Absolute paths from main and from hooks use the host's form: '/' on Unix, '\' (or '/') with a drive letter and
// case-insensitive names on Windows. Every function takes the platform as a flag so the tests run on any host.

function normalized(path: string, windows: boolean): string {
  const slashed = windows ? path.replace(/\\/g, '/').toLowerCase() : path
  return slashed.length > 1 ? slashed.replace(/\/+$/, '') : slashed
}

export function isAbsolutePath(path: string, windows: boolean): boolean {
  if (path.startsWith('/')) return true
  return windows && (/^[a-zA-Z]:[\\/]/.test(path) || path.startsWith('\\\\'))
}

// True when path is root itself or below it.
export function pathWithin(root: string, path: string, windows: boolean): boolean {
  const r = normalized(root, windows)
  const p = normalized(path, windows)
  return p === r || p.startsWith(r.endsWith('/') ? r : r + '/')
}

// The part of path below root, '/'-separated, '' for root itself; null when path is not within root.
export function pathBelow(root: string, path: string, windows: boolean): string | null {
  if (!pathWithin(root, path, windows)) return null
  const rest = path.slice(root.replace(/[\\/]+$/, '').length)
  return (windows ? rest.replace(/\\/g, '/') : rest).replace(/^\/+/, '')
}

export function lastSegment(path: string, windows: boolean): string {
  const parts = (windows ? path.split(/[\\/]/) : path.split('/')).filter(Boolean)
  return parts.pop() ?? path
}
