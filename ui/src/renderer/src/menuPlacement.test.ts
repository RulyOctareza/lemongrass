// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import assert from 'node:assert/strict'
import test from 'node:test'
import { MENU_MARGIN, placeMenu, pointAnchor } from './menuPlacement.ts'

const viewport = { width: 1000, height: 800 }
const menu = { width: 240, height: 300 }

test('a menu with room opens right of and below the pointer', () => {
  assert.deepEqual(placeMenu(pointAnchor(100, 100), menu, viewport), { left: 100, top: 100 })
})

test('near the right edge it grows to the left of the pointer at full width', () => {
  assert.deepEqual(placeMenu(pointAnchor(900, 100), menu, viewport), { left: 660, top: 100 })
})

test('near the bottom edge it grows upward from the pointer', () => {
  assert.deepEqual(placeMenu(pointAnchor(100, 700), menu, viewport), { left: 100, top: 400 })
})

test('a button menu flips above the button, not above its bottom edge', () => {
  const anchor = { x: { start: 50, end: 80 }, y: { start: 704, end: 676 } }
  assert.deepEqual(placeMenu(anchor, menu, viewport), { left: 50, top: 376 })
})

test('a right-aligned button menu keeps its right edge on the button', () => {
  const anchor = {
    x: { start: 300, end: 320, prefer: 'backward' as const },
    y: pointAnchor(0, 40).y
  }
  assert.equal(placeMenu(anchor, menu, viewport).left, 80)
})

test('a right-aligned menu with no room on the left opens forward', () => {
  const anchor = { x: { start: 10, end: 30, prefer: 'backward' as const }, y: pointAnchor(0, 40).y }
  assert.equal(placeMenu(anchor, menu, viewport).left, 10)
})

test('a menu that fits on neither side stays inside the window', () => {
  const tall = { width: 240, height: 700 }
  const { top } = placeMenu(pointAnchor(100, 400), tall, viewport)
  assert.ok(top >= MENU_MARGIN && top + tall.height <= viewport.height - MENU_MARGIN)
})
