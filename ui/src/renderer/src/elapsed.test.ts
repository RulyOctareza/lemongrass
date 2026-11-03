// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { formatElapsed } from './elapsed.ts'

test('elapsed time reads like a clock', () => {
  assert.equal(formatElapsed(0), '0:00')
  assert.equal(formatElapsed(7_400), '0:07')
  assert.equal(formatElapsed(754_000), '12:34')
  assert.equal(formatElapsed(3_723_000), '1:02:03')
  assert.equal(formatElapsed(-5_000), '0:00')
})
