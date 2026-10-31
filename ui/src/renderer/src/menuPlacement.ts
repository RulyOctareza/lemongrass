// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Where a popup menu opens from on one axis. It prefers to occupy [start, start + size], growing
// forward, or [end - size, end], growing backward, and takes the other side when the preferred one
// has no room. A pointer is a single coordinate for both; a button is its two edges, so a flipped
// menu still lines up with it.
export interface MenuAxis {
  start: number
  end: number
  prefer?: 'forward' | 'backward'
}

export interface MenuAnchor {
  x: MenuAxis
  y: MenuAxis
}

export interface MenuSize {
  width: number
  height: number
}

// Distance kept from the window's edges.
export const MENU_MARGIN = 4

export function pointAnchor(x: number, y: number): MenuAnchor {
  return { x: { start: x, end: x }, y: { start: y, end: y } }
}

function placeAxis(axis: MenuAxis, size: number, limit: number): number {
  const forward = axis.start
  const backward = axis.end - size
  const fitsForward = forward + size <= limit - MENU_MARGIN
  const fitsBackward = backward >= MENU_MARGIN
  let pos: number
  if (axis.prefer === 'backward') pos = fitsBackward || !fitsForward ? backward : forward
  else pos = fitsForward || !fitsBackward ? forward : backward
  if (!fitsForward && !fitsBackward) {
    // Neither side fits: take the roomier one, then the clamp below keeps it in the window.
    pos = limit - MENU_MARGIN - axis.start >= axis.end - MENU_MARGIN ? forward : backward
  }
  return Math.max(MENU_MARGIN, Math.min(pos, limit - size - MENU_MARGIN))
}

// The menu's top-left corner for its measured size, always inside the window.
export function placeMenu(
  anchor: MenuAnchor,
  size: MenuSize,
  viewport: MenuSize
): { left: number; top: number } {
  return {
    left: placeAxis(anchor.x, size.width, viewport.width),
    top: placeAxis(anchor.y, size.height, viewport.height)
  }
}
