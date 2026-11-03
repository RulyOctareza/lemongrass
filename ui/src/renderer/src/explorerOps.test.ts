// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { formatSize, gitMarksByPath } from './explorerOps.ts'

test('sizes read in the nearest unit', () => {
  assert.equal(formatSize(0), '0 B')
  assert.equal(formatSize(1023), '1023 B')
  assert.equal(formatSize(1536), '1.5 KB')
  assert.equal(formatSize(20 * 1024), '20 KB')
  assert.equal(formatSize(3 * 1024 * 1024), '3.0 MB')
})

test('a folder carries the most pressing change below it', () => {
  const marks = gitMarksByPath([
    { path: 'src/a/one.ts', group: 'changed' },
    { path: 'src/b/two.ts', group: 'unstagedNew' },
    { path: 'docs/x.md', group: 'deleted' }
  ])
  assert.equal(marks.get('src/a/one.ts'), 'changed')
  assert.equal(marks.get('src/a'), 'changed')
  assert.equal(marks.get('src'), 'unstagedNew')
  assert.equal(marks.get('docs'), 'deleted')
  assert.equal(marks.get('other'), undefined)
})

test('a project inside a repository keeps only its own files, prefix stripped', () => {
  const marks = gitMarksByPath(
    [
      { path: 'ui/src/a.ts', group: 'changed' },
      { path: 'go/main.go', group: 'changed' }
    ],
    'ui/'
  )
  assert.equal(marks.get('src/a.ts'), 'changed')
  assert.equal(marks.get('src'), 'changed')
  assert.equal(marks.has('go/main.go'), false)
  assert.equal(marks.size, 2)
})
