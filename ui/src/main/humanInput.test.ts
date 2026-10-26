// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
// @ts-ignore
import assert from 'node:assert/strict'
// @ts-ignore
import { classifyInput } from './humanInput.ts'

test('terminal replies are not typing', () => {
  for (const data of [
    '\x1b[I',
    '\x1b[O',
    '\x1b[12;40R',
    '\x1b[?1;2c',
    '\x1b[0n',
    '\x1b[?2004;1$y',
    '\x1b]11;rgb:0000/0000/0000\x07',
    '\x1b]10;rgb:ffff/ffff/ffff\x1b\\',
    '\x1b[<0;10;5M',
    '\x1b[I\x1b[12;40R'
  ]) {
    assert.equal(classifyInput(data), 'reply', JSON.stringify(data))
  }
})

test('a plain Enter ends the draft', () => {
  assert.equal(classifyInput('\r'), 'enter')
})

test('keys and pastes that start with an escape byte count as typing', () => {
  for (const data of [
    'a',
    '\x7f',
    '\x1b\r',
    '\x1b[A',
    '\x1b[3~',
    '\x1b',
    '\x1b[200~pasted text\x1b[201~',
    '\x1b[I\x1b[A'
  ]) {
    assert.equal(classifyInput(data), 'typing', JSON.stringify(data))
  }
})
