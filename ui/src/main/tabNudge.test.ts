// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import assert from 'node:assert/strict'
import { readFileSync } from 'node:fs'
import { test } from 'node:test'
import { runInNewContext } from 'node:vm'
import { ModuleKind, ScriptTarget, transpileModule } from 'typescript'

const TAB_ID = 'aaaaaaaa-1111-4111-8111-111111111111'
const NOTICE = '[lg] thread abc123: 1 for you from lead'
const source = readFileSync(new URL('./tabNudge.ts', import.meta.url), 'utf8')
const compiled = transpileModule(source, {
  compilerOptions: { module: ModuleKind.CommonJS, target: ScriptTarget.ES2022 }
})

function fixture(
  command: string,
  humanTyping = false,
  notice = NOTICE
): {
  handleNudge: (
    path: string,
    tabId: string
  ) => Promise<{ ok: boolean; typed?: boolean; reason?: string }>
  writes: string[]
  timers: { callback: () => void; delay: number }[]
  composed: () => number
} {
  const writes: string[] = []
  const timers: { callback: () => void; delay: number }[] = []
  let composed = 0
  const exports: {
    handleNudge?: (
      path: string,
      tabId: string
    ) => Promise<{ ok: boolean; typed?: boolean; reason?: string }>
  } = {}
  runInNewContext(compiled.outputText, {
    exports,
    require: (name: string) => {
      if (name === './pty') {
        return {
          findShellByTab: () => ({
            command,
            cwd: '/tmp',
            humanIsTyping: () => humanTyping,
            type: (text: string) => writes.push(text)
          })
        }
      }
      if (name === 'child_process') {
        return {
          execFile: (
            _path: string,
            args: string[],
            _options: unknown,
            done: (failure: Error | null, stdout: string) => void
          ) => {
            assert.deepEqual(Array.from(args), ['nudge', TAB_ID])
            composed++
            done(null, notice)
          }
        }
      }
      throw new Error(`Unexpected module: ${name}`)
    },
    setTimeout: (callback: () => void, delay: number) => {
      timers.push({ callback, delay })
    }
  })
  return { handleNudge: exports.handleNudge!, writes, timers, composed: () => composed }
}

test('Codex writes the notice before a separately delayed Enter', async () => {
  const f = fixture('codex')
  const result = f.handleNudge('/tmp/lgrassd', TAB_ID)
  await Promise.resolve()
  assert.deepEqual(f.writes, [NOTICE])
  assert.equal(f.timers.length, 1)
  assert.equal(f.timers[0].delay, 50)
  f.timers[0].callback()
  assert.equal((await result).typed, true)
  assert.deepEqual(f.writes, [NOTICE, '\r'])
})

test('Claude gets the same separately delayed Enter', async () => {
  const f = fixture('claude')
  const result = f.handleNudge('/tmp/lgrassd', TAB_ID)
  await Promise.resolve()
  assert.deepEqual(f.writes, [NOTICE])
  assert.equal(f.timers.length, 1)
  assert.equal(f.timers[0].delay, 50)
  f.timers[0].callback()
  assert.equal((await result).typed, true)
  assert.deepEqual(f.writes, [NOTICE, '\r'])
})

test('human typing defers the notice before consuming or writing it', async () => {
  const f = fixture('codex', true)
  assert.equal((await f.handleNudge('/tmp/lgrassd', TAB_ID)).reason, 'typing')
  assert.equal(f.composed(), 0)
  assert.deepEqual(f.writes, [])
  assert.equal(f.timers.length, 0)
})

test('an empty ledger produces no text or Enter', async () => {
  const f = fixture('codex', false, '')
  assert.equal((await f.handleNudge('/tmp/lgrassd', TAB_ID)).reason, 'nothing pending')
  assert.deepEqual(f.writes, [])
  assert.equal(f.timers.length, 0)
})
