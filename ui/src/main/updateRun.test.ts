// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { runUntilExec } from './updateRun.ts'

test('calls onExec once the process execs another program', async () => {
  let calls = 0
  await runUntilExec('sh', ['-c', 'sleep 0.4; exec sleep 0.4'], () => calls++, 50)
  assert.equal(calls, 1)
})

test('calls onExec when the exec happens before the first poll', async () => {
  let calls = 0
  await runUntilExec('sh', ['-c', 'exec sleep 0.3'], () => calls++, 50)
  assert.equal(calls, 1)
})

test('does not call onExec when the process exits without exec', async () => {
  let calls = 0
  await runUntilExec('sh', ['-c', 'sleep 0.3; true'], () => calls++, 50)
  assert.equal(calls, 0)
})

test('rejects with the exit code and leaves onExec uncalled', async () => {
  let calls = 0
  await assert.rejects(
    runUntilExec('sh', ['-c', 'sleep 0.2; exit 126'], () => calls++, 50),
    (err: { code?: unknown }) => err.code === 126
  )
  assert.equal(calls, 0)
})
