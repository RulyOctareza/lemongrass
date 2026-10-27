// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
// @ts-ignore
import {
  keyMessage,
  scrollMessage,
  textMessages,
  touchMessage,
  TouchAction
} from './scrcpyControl.ts'

const point = { x: 100, y: 200, width: 736, height: 1600 }

test('a touch down is 32 bytes with a finger pointer, the frame size and full pressure', () => {
  const out = touchMessage(TouchAction.down, point)
  const view = new DataView(out.buffer)
  assert.equal(out.length, 32)
  assert.equal(out[0], 2)
  assert.equal(out[1], 0)
  assert.equal(view.getBigUint64(2), 0xfffffffffffffffen)
  assert.equal(view.getInt32(10), 100)
  assert.equal(view.getInt32(14), 200)
  assert.equal(view.getUint16(18), 736)
  assert.equal(view.getUint16(20), 1600)
  assert.equal(view.getUint16(22), 0xffff)
})

test('a touch up has no pressure', () => {
  const view = new DataView(touchMessage(TouchAction.up, point).buffer)
  assert.equal(view.getUint16(22), 0)
})

test('a key message is 14 bytes', () => {
  const out = keyMessage(1, 66)
  const view = new DataView(out.buffer)
  assert.deepEqual([out.length, out[0], out[1], view.getUint32(2)], [14, 0, 1, 66])
})

test('text is length prefixed and split under the server limit', () => {
  const [single] = textMessages('hé')
  assert.deepEqual([...single], [1, 0, 0, 0, 3, 0x68, 0xc3, 0xa9])
  const long = textMessages('é'.repeat(200))
  assert.equal(long.length, 2)
  for (const message of long) assert.ok(message.length - 5 <= 300)
})

test('scroll is normalized to a 16-bit fixed point', () => {
  const out = scrollMessage(point, 0, -16)
  const view = new DataView(out.buffer)
  assert.equal(out.length, 21)
  assert.equal(out[0], 3)
  assert.equal(view.getInt16(15), -0x8000)
  assert.equal(view.getInt16(13), 0)
})
