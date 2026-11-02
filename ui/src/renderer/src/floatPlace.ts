// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Places a floating element (a dropdown, a hint card) fixed to the window beside its anchor. A modal
// and most panes clip with overflow, so anything floating out of them renders in body and is placed
// here instead of being positioned inside its parent.

export interface FloatPlacement {
  left: number
  // Exactly one of top and bottom is set: an upward float is pinned by its bottom edge, so content
  // that shrinks, such as a filtered list, stays against the anchor.
  top?: number
  bottom?: number
  // The room on the chosen side; the float scrolls or clips past it.
  maxHeight: number
}

const EDGE = 8

// Opens below the anchor unless the float does not fit there and there is more room above;
// preferAbove flips that preference for an anchor near the bottom of the window.
export function placeFloat(
  anchor: DOMRect,
  width: number,
  height: number,
  gap: number,
  preferAbove = false
): FloatPlacement {
  const viewWidth = window.innerWidth
  const viewHeight = window.innerHeight
  const left = Math.max(EDGE, Math.min(anchor.left, viewWidth - width - EDGE))
  const below = viewHeight - anchor.bottom - gap - EDGE
  const above = anchor.top - gap - EDGE
  const up = preferAbove ? !(above < height && below > above) : below < height && above > below
  if (up) return { left, bottom: viewHeight - anchor.top + gap, maxHeight: Math.max(0, above) }
  return { left, top: anchor.bottom + gap, maxHeight: Math.max(0, below) }
}

export function placementStyle(p: FloatPlacement): Record<string, string> {
  return {
    left: `${p.left}px`,
    top: p.top !== undefined ? `${p.top}px` : 'auto',
    bottom: p.bottom !== undefined ? `${p.bottom}px` : 'auto',
    maxHeight: `${p.maxHeight}px`
  }
}
