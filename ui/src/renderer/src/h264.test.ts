// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
// @ts-ignore
import { avcCodecString, concatBytes } from './h264.ts'

test('reads the codec from the SPS of a scrcpy config packet', () => {
  const config = new Uint8Array([0, 0, 0, 1, 0x67, 0x64, 0x00, 0x20, 0xac, 0, 0, 0, 1, 0x68, 0xee])
  assert.equal(avcCodecString(config), 'avc1.640020')
})

test('finds the SPS after another NAL unit', () => {
  const data = new Uint8Array([0, 0, 1, 0x09, 0xf0, 0, 0, 1, 0x27, 0x42, 0xc0, 0x1f, 0x8d])
  assert.equal(avcCodecString(data), 'avc1.42c01f')
})

test('a packet without an SPS has no codec', () => {
  assert.equal(avcCodecString(new Uint8Array([0, 0, 0, 1, 0x65, 1, 2, 3, 4])), null)
})

test('concatBytes joins in order', () => {
  assert.deepEqual([...concatBytes(new Uint8Array([1, 2]), new Uint8Array([3]))], [1, 2, 3])
})
