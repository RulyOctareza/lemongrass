// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// The resolver's icons in one map, as GitIcon holds the git panel's, so a control is a name here
// and never its own inline SVG. Each is stroke path data in a 14 by 14 box.
export const CONFLICT_ICONS = {
  prev: ['M3.5 8.5L7 5l3.5 3.5'],
  next: ['M3.5 5.5L7 9l3.5-3.5'],
  // Two double chevrons meeting in the middle, as in compressing the file to its conflicts.
  focus: ['M4 1.5l3 2 3-2', 'M4 4l3 2 3-2', 'M4 10l3-2 3 2', 'M4 12.5l3-2 3 2'],
  more: [
    'M3 6.3a.7.7 0 1 1 0 1.4a.7.7 0 1 1 0-1.4',
    'M7 6.3a.7.7 0 1 1 0 1.4a.7.7 0 1 1 0-1.4',
    'M11 6.3a.7.7 0 1 1 0 1.4a.7.7 0 1 1 0-1.4'
  ],
  accept: ['M3 7.5l2.5 2.5L11 4.5'],
  // A take points from its side into the middle, as the decision gutter's chevron does: the left
  // ref's to the right, the right ref's to the left, and both meet. conflictEditor.ts keeps its
  // own copy of the two chevrons for the gutter.
  takeLeft: ['M3 3.5l3.5 3.5L3 10.5M7.5 3.5L11 7l-3.5 3.5'],
  takeRight: ['M11 3.5L7.5 7l3.5 3.5M6.5 3.5L3 7l3.5 3.5'],
  takeBoth: ['M2.5 3.5L6 7l-3.5 3.5', 'M11.5 3.5L8 7l3.5 3.5'],
  reset: ['M2 7a5 5 0 1 0 1.46-3.54L2 4.9', 'M2 2v2.9h2.9'],
  acceptAll: ['M1.5 7.5L4 10l5-5.5', 'M7 9.5l.5.5 5-5.5']
} as const satisfies Record<string, readonly string[]>

export type ConflictIconName = keyof typeof CONFLICT_ICONS
