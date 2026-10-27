// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { setTimeout as sleep } from 'node:timers/promises'
import { createNoticeQueue, type BranchNotice } from './gitNotice.ts'

function collector(): {
  sent: [string, BranchNotice][]
  send: (p: string, n: BranchNotice) => void
} {
  const sent: [string, BranchNotice][] = []
  return { sent, send: (projectPath, notice) => sent.push([projectPath, notice]) }
}

test('a burst of moves sends one notice from the first branch to the last', async () => {
  const { sent, send } = collector()
  const queue = createNoticeQueue(send, 30)
  queue.record('/p', { from: 'main', to: 'wrong', carry: 'carried' })
  await sleep(10)
  queue.record('/p', { from: 'wrong', to: 'right', carry: 'none' })
  await sleep(10)
  assert.deepEqual(sent, [])
  await sleep(40)
  assert.deepEqual(sent, [['/p', { from: 'main', to: 'right', carry: 'carried' }]])
})

test('a conflict anywhere in the burst wins over a clean carry', () => {
  const { sent, send } = collector()
  const queue = createNoticeQueue(send, 1000)
  queue.record('/p', { from: 'main', to: 'a', carry: 'conflict' })
  queue.record('/p', { from: 'a', to: 'b', carry: 'carried' })
  queue.flush()
  assert.deepEqual(sent, [['/p', { from: 'main', to: 'b', carry: 'conflict' }]])
})

test('a burst that ends where it started sends nothing', () => {
  const { sent, send } = collector()
  const queue = createNoticeQueue(send, 1000)
  queue.record('/p', { from: 'main', to: 'wrong', carry: 'none' })
  queue.record('/p', { from: 'wrong', to: 'main', carry: 'none' })
  queue.flush()
  assert.deepEqual(sent, [])
})

test('projects are debounced apart and flush sends every pending notice at once', () => {
  const { sent, send } = collector()
  const queue = createNoticeQueue(send, 1000)
  queue.record('/a', { from: 'main', to: 'x', carry: 'none' })
  queue.record('/b', { from: 'main', to: 'y', carry: 'none' })
  queue.flush()
  assert.deepEqual(sent, [
    ['/a', { from: 'main', to: 'x', carry: 'none' }],
    ['/b', { from: 'main', to: 'y', carry: 'none' }]
  ])
  queue.flush()
  assert.equal(sent.length, 2)
})
