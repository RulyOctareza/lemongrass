// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { isAbsolutePath, lastSegment, pathBelow, pathWithin } from './pathForm.ts'

test('unix paths compare exactly, by whole segment', () => {
  assert.ok(pathWithin('/home/a/proj', '/home/a/proj', false))
  assert.ok(pathWithin('/home/a/proj/', '/home/a/proj/ui', false))
  assert.ok(!pathWithin('/home/a/proj', '/home/a/project', false))
  assert.ok(!pathWithin('/home/a/Proj', '/home/a/proj', false))
  assert.ok(!pathWithin('/home/a/proj', 'C:\\home\\a\\proj', false))
})

test('windows paths compare across separators and case', () => {
  assert.ok(pathWithin('C:\\Users\\A\\proj', 'c:\\users\\a\\proj\\ui', true))
  assert.ok(pathWithin('C:\\Users\\A\\proj\\', 'C:/Users/A/proj', true))
  assert.ok(!pathWithin('C:\\Users\\A\\proj', 'C:\\Users\\A\\project', true))
  assert.ok(pathWithin('C:\\', 'C:\\x', true))
})

test('the part below the root is slash separated', () => {
  assert.equal(pathBelow('C:\\Users\\A\\proj', 'c:\\users\\a\\proj\\ui\\src', true), 'ui/src')
  assert.equal(pathBelow('/home/a/proj', '/home/a/proj', false), '')
  assert.equal(pathBelow('/home/a/proj/', '/home/a/proj/ui', false), 'ui')
  assert.equal(pathBelow('/home/a/proj', '/home/a/other', false), null)
})

test('absolute forms and last segments per platform', () => {
  assert.ok(isAbsolutePath('/tmp/a.png', false))
  assert.ok(!isAbsolutePath('C:\\a.png', false))
  assert.ok(isAbsolutePath('C:\\a.png', true))
  assert.ok(isAbsolutePath('\\\\server\\share\\a.png', true))
  assert.ok(!isAbsolutePath('a.png', true))
  assert.equal(lastSegment('C:\\Users\\A\\proj\\', true), 'proj')
  assert.equal(lastSegment('/home/a/we\\ird', false), 'we\\ird')
})
