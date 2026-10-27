// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { GUIDE_CARDS } from './guide.ts'

test('every card has all fields filled in', () => {
  for (const card of GUIDE_CARDS) {
    for (const [field, value] of Object.entries(card)) {
      assert.equal(typeof value, 'string', `${card.id}.${field} is not a string`)
      assert.notEqual(value.trim(), '', `${card.id}.${field} is empty`)
    }
  }
})

test('card ids are unique', () => {
  const ids = GUIDE_CARDS.map((card) => card.id)
  assert.equal(new Set(ids).size, ids.length)
})

test('guide copy has no em dashes or double hyphens', () => {
  for (const card of GUIDE_CARDS) {
    for (const value of Object.values(card)) {
      assert.doesNotMatch(value, /—|--/, `${card.id} copy has a dash: ${value}`)
    }
  }
})
