// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { buildSearchTree, fuzzyMatch, rangesWithin } from './treeSearch.ts'

test('a query matches its characters in order, case-insensitive, spaces ignored', () => {
  assert.ok(fuzzyMatch('src/main/projectFiles.ts', 'PF'))
  assert.ok(fuzzyMatch('src/main/projectFiles.ts', 'proj files'))
  assert.equal(fuzzyMatch('src/main/projectFiles.ts', 'zz'), null)
  assert.equal(fuzzyMatch('src/a.ts', '   '), null)
})

test('a match inside the file name outranks a match spread over folders', () => {
  const name = fuzzyMatch('deep/nested/folder/table.ts', 'table')!
  const spread = fuzzyMatch('t/a/b/l/e/x.ts', 'table')!
  assert.ok(name.score > spread.score)
})

test('an exact name ranks above a longer name that contains it', () => {
  const exact = fuzzyMatch('ui/src/app.ts', 'app')!
  const longer = fuzzyMatch('ui/src/wrapper.ts', 'app')!
  assert.ok(exact.score > longer.score)
})

test('ranges merge consecutive characters and point into the full path', () => {
  const match = fuzzyMatch('src/tabKinds.ts', 'kinds')!
  assert.deepEqual(match.ranges, [[7, 12]])
  assert.equal('src/tabKinds.ts'.slice(7, 12), 'Kinds')
})

test('a query with a slash matches across folders', () => {
  const match = fuzzyMatch('ui/src/main/index.ts', 'main/ind')!
  assert.deepEqual(match.ranges, [[7, 15]])
})

test('rangesWithin clips and shifts ranges to one segment', () => {
  assert.deepEqual(
    rangesWithin(
      [
        [0, 2],
        [5, 9]
      ],
      4,
      7
    ),
    [[1, 3]]
  )
  assert.deepEqual(rangesWithin([[0, 2]], 3, 6), [])
})

test('the search tree holds matches and their folders, best branch first', () => {
  const result = buildSearchTree(
    [
      { path: 'docs/guide.md', type: 'file' },
      { path: 'src/components/Button.vue', type: 'file' },
      { path: 'src/components/ButtonGroup.vue', type: 'file' },
      { path: 'src/util.ts', type: 'file' }
    ],
    'button'
  )
  assert.deepEqual(
    result.rows.map((row) => [row.node.path, row.depth, row.node.match]),
    [
      ['src', 0, false],
      ['src/components', 1, false],
      ['src/components/Button.vue', 2, true],
      ['src/components/ButtonGroup.vue', 2, true]
    ]
  )
  assert.deepEqual(result.rows[2].node.ranges, [[0, 6]])
  assert.equal(result.hidden, 0)
})

test('a folder that matches keeps its own ranges when it also holds a match', () => {
  const result = buildSearchTree(
    [
      { path: 'tests', type: 'dir' },
      { path: 'tests/test.ts', type: 'file' }
    ],
    'test'
  )
  const folder = result.rows.find((row) => row.node.path === 'tests')!.node
  assert.equal(folder.match, true)
  assert.equal(folder.type, 'dir')
  assert.deepEqual(folder.ranges, [[0, 4]])
})

test('matches beyond the cap are counted, not kept', () => {
  const candidates = Array.from({ length: 5 }, (_, i) => ({
    path: `f${i}.ts`,
    type: 'file' as const
  }))
  const result = buildSearchTree(candidates, 'f', 3)
  assert.equal(result.rows.length, 3)
  assert.equal(result.hidden, 2)
})
