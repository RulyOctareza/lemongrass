// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { parseBlame, parseLog, parseNameStatus, parseStatus, parseTrack } from './gitParse.ts'

const HASH = '0000000000000000000000000000000000000000'
const mode = '100644 100644 100644'

function ordinary(xy: string, path: string): string {
  return `1 ${xy} N... ${mode} ${HASH} ${HASH} ${path}`
}

test('groups files by what they are and whether they are staged', () => {
  const raw = [
    '# branch.oid abc',
    '# branch.head main',
    ordinary('.M', 'a-modified.ts'),
    ordinary('A.', 'b-staged-new.ts'),
    ordinary('AM', 'c-staged-new-edited.ts'),
    ordinary('.D', 'd-deleted.ts'),
    ordinary('D.', 'e-staged-deleted.ts'),
    '? f-untracked.ts',
    `u UU N... ${mode} 100644 ${HASH} ${HASH} ${HASH} g-conflict.ts`,
    `2 R. N... ${mode} ${HASH} ${HASH} R100 h-renamed.ts\0h-old.ts`
  ].join('\0')
  const groups = Object.fromEntries(parseStatus(raw).files.map((file) => [file.path, file.group]))
  assert.deepEqual(groups, {
    'a-modified.ts': 'changed',
    'b-staged-new.ts': 'stagedNew',
    'c-staged-new-edited.ts': 'stagedNew',
    'd-deleted.ts': 'deleted',
    'e-staged-deleted.ts': 'deleted',
    'f-untracked.ts': 'unstagedNew',
    'g-conflict.ts': 'conflicted',
    'h-renamed.ts': 'renamed'
  })
})

test('reads name-status records with renames and copies carrying two paths', () => {
  const raw = ['M', 'src/a.ts', 'R100', 'old.ts', 'new.ts', 'A', 'b.ts', 'D', 'c.ts', ''].join('\0')
  assert.deepEqual(parseNameStatus(raw), [
    { path: 'b.ts', kind: 'added' },
    { path: 'c.ts', kind: 'deleted' },
    { path: 'new.ts', origPath: 'old.ts', kind: 'renamed' },
    { path: 'src/a.ts', kind: 'modified' }
  ])
})

test('reads log records with ref labels and the unpushed mark', () => {
  const names = { local: new Set(['main', 'feature/x']), remote: new Set(['origin/main']) }
  const record = (fields: string[]): string => `${fields.join('\x1f')}\x1e\n`
  const raw =
    record([
      'aaa',
      'bbb ccc',
      'Ann',
      '100',
      'HEAD -> main, origin/main, tag: v1, feature/x',
      'merge'
    ]) + record(['bbb', '', 'Bob', '90', '', 'root'])
  const [first, second] = parseLog(raw, names, new Set(['aaa']))
  assert.deepEqual(first.parents, ['bbb', 'ccc'])
  assert.equal(first.unpushed, true)
  assert.deepEqual(first.refs, [
    { name: 'HEAD', kind: 'head' },
    { name: 'main', kind: 'branch' },
    { name: 'origin/main', kind: 'remote' },
    { name: 'v1', kind: 'tag' },
    { name: 'feature/x', kind: 'branch' }
  ])
  assert.deepEqual(second.parents, [])
  assert.equal(second.unpushed, false)
})

test('reads blame output where later lines of a commit carry no details', () => {
  const h1 = 'a'.repeat(40)
  const h2 = 'b'.repeat(40)
  const raw = [
    `${h1} 1 1 2`,
    'author Ann',
    'author-time 100',
    'summary first',
    '\tone',
    `${h1} 2 2`,
    '\ttwo',
    `${h2} 1 3 1`,
    'author Bob',
    'author-time 200',
    '\tthree',
    ''
  ].join('\n')
  assert.deepEqual(
    parseBlame(raw).map((line) => [line.number, line.hash[0], line.author, line.time, line.text]),
    [
      [1, 'a', 'Ann', 100, 'one'],
      [2, 'a', 'Ann', 100, 'two'],
      [3, 'b', 'Bob', 200, 'three']
    ]
  )
})

test('reads ahead and behind from an upstream track', () => {
  assert.deepEqual(parseTrack('[ahead 2, behind 1]'), { ahead: 2, behind: 1 })
  assert.deepEqual(parseTrack('[behind 3]'), { ahead: 0, behind: 3 })
  assert.deepEqual(parseTrack('[gone]'), { ahead: 0, behind: 0 })
  assert.deepEqual(parseTrack(''), { ahead: 0, behind: 0 })
})
