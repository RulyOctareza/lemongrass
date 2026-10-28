// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { GROUP_COLORS, defaultGroupColor, groupColorVar, isGroupColor } from './groupColors.ts'

test('consecutive group ids get different default colors', () => {
  const first = defaultGroupColor(1)
  assert.notEqual(first, defaultGroupColor(2))
  assert.equal(defaultGroupColor(1 + GROUP_COLORS.length), first)
})

test('default color is a palette color for any id', () => {
  for (const id of [0, 1, 7, 1000, -3]) assert.ok(isGroupColor(defaultGroupColor(id)))
})

test('unknown names are not palette colors', () => {
  assert.equal(isGroupColor('amber'), false)
  assert.equal(isGroupColor(undefined), false)
})

test('color maps to its Sankasten token', () => {
  assert.equal(groupColorVar('jade'), 'var(--color-jade)')
})
