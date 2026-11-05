// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// The shapes a blocked path may take in the Safety section before it is saved; the gate's own validation (guard.Policy.Validate) stays the judge. Windows also takes a drive path, a ~\ path, a UNC path and the home variables, which the gate normalizes.
const windowsShapes = [/^[A-Za-z]:[\\/]/, /^~\\/, /^\\\\/, /^%USERPROFILE%([\\/]|$)/i, /^\$env:USERPROFILE([\\/]|$)/i]

export function policyPathError(value: string, windows: boolean): string {
  if (value === '~' || value.startsWith('~/') || value.startsWith('/')) return ''
  if (windows && windowsShapes.some((shape) => shape.test(value))) return ''
  return windows ? 'Start the path with a drive such as C:\\ or with ~\\.' : 'Start the path with / or ~/.'
}
