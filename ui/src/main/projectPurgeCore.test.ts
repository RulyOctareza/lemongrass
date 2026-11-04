// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
// @ts-ignore
import { PURGE_AFTER_MS, expiredProjectIds } from './projectPurgeCore.ts'

const now = Date.parse('2026-11-04T00:00:00.000Z')

test('expiredProjectIds returns only projects deleted at least 30 days ago', () => {
  const projects = [
    { id: 'live' },
    { id: 'recent', deletedAt: new Date(now - PURGE_AFTER_MS + 1000).toISOString() },
    { id: 'due', deletedAt: new Date(now - PURGE_AFTER_MS).toISOString() },
    { id: 'old', deletedAt: '2026-01-01T00:00:00.000Z' }
  ]
  assert.deepEqual(expiredProjectIds(projects, now), ['due', 'old'])
})

test('expiredProjectIds never expires a deletedAt that does not parse', () => {
  assert.deepEqual(expiredProjectIds([{ id: 'odd', deletedAt: 'yesterday-ish' }], now), [])
})
