// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { IconShape } from './tabKinds'

export interface AppGlyph {
  // The viewBox, square or width by height.
  box: number | readonly [number, number]
  // null draws the shapes filled instead of stroked.
  stroke: number | null
  shapes: IconShape[]
}

const path = (d: string): IconShape => ({ tag: 'path', attrs: { d } })
const dot = (cx: number, cy: number, r: number): IconShape => ({
  tag: 'circle',
  attrs: { cx, cy, r }
})

// Glyphs that mean the same thing on any screen, drawn by AppIcon.vue. A surface's own icons, such
// as the git panel's or the resolver's, stay in that surface's set.
export const APP_ICONS = {
  trash: {
    box: 14,
    stroke: 1.2,
    shapes: [path('M2.5 4h9'), path('M5.5 4V2.5h3V4'), path('M3.5 4l.5 7.5h6L10.5 4')]
  },
  chevronRight: { box: 10, stroke: 1.2, shapes: [path('M3 2l3 3-3 3')] },
  chevronDown: { box: 10, stroke: 1.4, shapes: [path('M2 3.5L5 6.5L8 3.5')] },
  close: { box: 14, stroke: 1.2, shapes: [path('M3.5 3.5l7 7M10.5 3.5l-7 7')] },
  plus: { box: 14, stroke: 1.2, shapes: [path('M7 2.5v9M2.5 7h9')] },
  check: { box: 14, stroke: 1.2, shapes: [path('M3 7.5l2.5 2.5L11 4.5')] },
  copy: {
    box: 14,
    stroke: 1.2,
    shapes: [
      { tag: 'rect', attrs: { x: 4.5, y: 4.5, width: 7, height: 8, rx: 1.2 } },
      path('M9.5 4.5V3.2A1.2 1.2 0 0 0 8.3 2H3.2A1.2 1.2 0 0 0 2 3.2v5.1a1.2 1.2 0 0 0 1.2 1.2h1.3')
    ]
  },
  search: { box: 16, stroke: 1.5, shapes: [dot(7, 7, 4.5), path('M10.5 10.5L14 14')] },
  info: {
    box: 16,
    stroke: 1.4,
    shapes: [
      dot(8, 8, 6.5),
      path('M8 7.2V11.2'),
      { tag: 'circle', attrs: { cx: 8, cy: 4.9, r: 0.4, fill: 'currentColor' } }
    ]
  },
  spinner: { box: 14, stroke: 1.6, shapes: [path('M7 2a5 5 0 1 1-5 5')] },
  dragHandle: {
    box: [10, 14],
    stroke: null,
    shapes: [
      dot(3, 3, 1.2),
      dot(7, 3, 1.2),
      dot(3, 7, 1.2),
      dot(7, 7, 1.2),
      dot(3, 11, 1.2),
      dot(7, 11, 1.2)
    ]
  }
} satisfies Record<string, AppGlyph>

export type AppIconName = keyof typeof APP_ICONS
