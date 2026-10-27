// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { cleanThinkerAccounts } from './accountNames.ts'

test('keeps names and primary entries', () => {
  assert.deepEqual(cleanThinkerAccounts(['', 'work', 'side-2'], 3), ['', 'work', 'side-2'])
})

test('treats a missing list as no choice', () => {
  assert.equal(cleanThinkerAccounts(undefined, 2), undefined)
})

test('refuses a list that does not match the members', () => {
  assert.equal(cleanThinkerAccounts(['work'], 2), null)
  assert.equal(cleanThinkerAccounts('work', 1), null)
})

test('refuses names outside the account rule', () => {
  assert.equal(cleanThinkerAccounts(['../x'], 1), null)
  assert.equal(cleanThinkerAccounts(['Work'], 1), null)
  assert.equal(cleanThinkerAccounts([7], 1), null)
})
