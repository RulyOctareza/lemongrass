// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import assert from 'node:assert/strict'
import { test } from 'node:test'
import { Capture } from './capture.ts'
import { notSettledText, settledText, settleVerdict } from './settle.ts'

const base = { loading: false, ages: [] as number[], quiet: 500, quietNeeded: 300 }

test('settled when nothing is loading, nothing is in flight and the page is quiet', () => {
  const verdict = settleVerdict(base)
  assert.equal(verdict.settled, true)
  assert.equal(settledText(verdict), 'settled')
})

test('a request open for over two seconds is background and does not hold the action', () => {
  const verdict = settleVerdict({ ...base, ages: [30_000, 2_500] })
  assert.equal(verdict.settled, true)
  assert.equal(verdict.background, 2)
  assert.equal(settledText(verdict), 'settled (ignored 2 background requests open for over 2s)')
})

test('a young request, a loading page or a changing page is not settled', () => {
  assert.equal(settleVerdict({ ...base, ages: [300] }).settled, false)
  assert.equal(settleVerdict({ ...base, loading: true }).settled, false)
  assert.equal(settleVerdict({ ...base, quiet: 100 }).settled, false)
})

test('the cap message names only what was really going on', () => {
  const input = { ...base, quiet: 40, ages: [200, 9_000] }
  const verdict = settleVerdict(input)
  assert.equal(
    notSettledText(input, verdict, 10),
    'not settled after 10s: 1 request(s) still in flight, the page changed 40 ms ago (ignored 1 background request open for over 2s)'
  )
})

test('capture reports how long each open request has been in flight', () => {
  const capture = new Capture({ send: async () => ({}) as never }, () => ({
    tokenPlacement: null,
    tokenPath: ''
  }))
  const start = (requestId: string, type: string): void =>
    capture.handle('Network.requestWillBeSent', {
      requestId,
      type,
      request: { url: 'http://h/x', method: 'GET', headers: {} }
    })
  start('a', 'XHR')
  start('b', 'WebSocket')
  const ages = capture.inFlightAges(Date.now() + 5_000)
  assert.equal(ages.length, 1)
  assert.ok(ages[0] >= 5_000)
  capture.handle('Network.loadingFinished', { requestId: 'a' })
  assert.deepEqual(capture.inFlightAges(), [])
})
