// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { beforeEach, afterEach, mock, test } from 'node:test'
import assert from 'node:assert/strict'
import { CONFIRM_DELAY_MS, createClickConfirm, type ConfirmState } from './clickConfirm.ts'

let seen: ConfirmState[]

beforeEach(() => {
  seen = []
  mock.timers.enable({ apis: ['setTimeout'] })
})

afterEach(() => mock.timers.reset())

test('the first press waits, then arms, and the armed press runs', () => {
  const confirm = createClickConfirm((s) => seen.push(s))
  assert.equal(confirm.press(), false)
  assert.equal(confirm.state(), 'pending')
  mock.timers.tick(CONFIRM_DELAY_MS)
  assert.equal(confirm.state(), 'armed')
  assert.equal(confirm.press(), true)
  assert.equal(confirm.state(), 'idle')
  assert.deepEqual(seen, ['pending', 'armed', 'idle'])
})

test('presses during the wait are ignored and do not restart it', () => {
  const confirm = createClickConfirm((s) => seen.push(s))
  confirm.press()
  mock.timers.tick(CONFIRM_DELAY_MS - 1)
  assert.equal(confirm.press(), false)
  mock.timers.tick(1)
  assert.equal(confirm.state(), 'armed')
})

test('a double fired click never runs the action', () => {
  const confirm = createClickConfirm(() => {})
  assert.equal(confirm.press(), false)
  assert.equal(confirm.press(), false)
  assert.equal(confirm.state(), 'pending')
})

test('reset during the wait cancels the arming', () => {
  const confirm = createClickConfirm((s) => seen.push(s))
  confirm.press()
  confirm.reset()
  mock.timers.tick(CONFIRM_DELAY_MS)
  assert.equal(confirm.state(), 'idle')
  assert.deepEqual(seen, ['pending', 'idle'])
})
