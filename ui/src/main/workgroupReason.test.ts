// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cleanReason, MAX_REASON_CHARS } from './workgroupReason.ts'

test('collapses whitespace and trims', () => {
  assert.equal(cleanReason('  use\n\nhaiku \t please '), 'use haiku please')
})

test('drops empty and non string input', () => {
  assert.equal(cleanReason('   \n'), undefined)
  assert.equal(cleanReason(undefined), undefined)
  assert.equal(cleanReason(42), undefined)
})

test('caps the length', () => {
  assert.equal(cleanReason('x'.repeat(MAX_REASON_CHARS + 50))?.length, MAX_REASON_CHARS)
})
