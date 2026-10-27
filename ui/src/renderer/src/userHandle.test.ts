// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { collidingUserName, userHandle } from './userHandle.ts'

test('a handle is trimmed, lowercased and joined with underscores', () => {
  assert.equal(userHandle('John Doe'), 'john_doe')
  assert.equal(userHandle('  john   DOE '), 'john_doe')
  assert.equal(userHandle('john_doe'), 'john_doe')
  assert.equal(userHandle('Alice'), 'alice')
  assert.equal(userHandle('tab\tseparated'), 'tab_separated')
  assert.equal(userHandle(''), '')
})

test('a user colliding with another is reported with the other name', () => {
  const names = ['John Doe', 'bob', 'john_doe']
  assert.equal(collidingUserName(names, 0), 'john_doe')
  assert.equal(collidingUserName(names, 2), 'John Doe')
  assert.equal(collidingUserName(names, 1), null)
})

test('blank names never collide', () => {
  assert.equal(collidingUserName(['', ' '], 0), null)
})
