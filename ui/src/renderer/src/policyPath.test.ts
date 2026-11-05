// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { policyPathError } from './policyPath.ts'

test('the POSIX shapes pass on every platform', () => {
  for (const windows of [false, true]) {
    for (const value of ['~', '~/private', '/etc/x']) {
      assert.equal(policyPathError(value, windows), '')
    }
  }
})

test('Windows shapes pass only on Windows', () => {
  for (const value of ['C:\\Users\\x\\secret', 'd:/data', '~\\private', '\\\\server\\share', '%USERPROFILE%\\x', '$env:USERPROFILE\\x']) {
    assert.equal(policyPathError(value, true), '')
    assert.notEqual(policyPathError(value, false), '')
  }
})

test('a relative path is refused', () => {
  assert.equal(policyPathError('private', false), 'Start the path with / or ~/.')
  assert.equal(policyPathError('private', true), 'Start the path with a drive such as C:\\ or with ~\\.')
})
