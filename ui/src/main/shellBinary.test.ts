// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

// @ts-ignore
import assert from 'node:assert/strict'
import { chmodSync, mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
// @ts-ignore
import { resolveShellBinary } from './shellBinary.ts'

function binDir(...names: string[]): string {
  const dir = mkdtempSync(join(tmpdir(), 'shellbin-'))
  for (const name of names) {
    const file = join(dir, name)
    writeFileSync(file, '#!/bin/sh\n')
    chmodSync(file, 0o755)
  }
  return dir
}

test('a command other than zsh is left alone', () => {
  assert.equal(resolveShellBinary('claude', { SHELL: '/bin/bash', PATH: '' }), 'claude')
})

test('the login shell is used when it is executable', () => {
  const dir = binDir('fish')
  const shell = join(dir, 'fish')
  assert.equal(resolveShellBinary('zsh', { SHELL: shell, PATH: '' }), shell)
})

test('zsh on PATH is used when SHELL is missing or not executable', () => {
  const dir = binDir('zsh', 'bash')
  assert.equal(resolveShellBinary('zsh', { SHELL: '/nope/zsh', PATH: dir }), 'zsh')
  assert.equal(resolveShellBinary('zsh', { PATH: dir }), 'zsh')
})

test('bash is the fallback when there is no zsh', () => {
  const dir = binDir('bash', 'sh')
  assert.equal(resolveShellBinary('zsh', { PATH: dir }), 'bash')
})

test('sh is the last fallback', () => {
  const dir = binDir('sh')
  assert.equal(resolveShellBinary('zsh', { PATH: dir }), 'sh')
})
