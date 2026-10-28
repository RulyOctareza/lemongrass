// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { WRITE_TIMEOUT_MS, readEnv, run } from './gitRun.ts'
import { moveBranch, type GitRunner } from './gitMove.ts'

// The machine's own git config, such as commit signing or hooks, must not reach these repos.
process.env.GIT_CONFIG_GLOBAL = '/dev/null'
process.env.GIT_CONFIG_NOSYSTEM = '1'
process.env.GIT_AUTHOR_NAME = process.env.GIT_COMMITTER_NAME = 'Test'
process.env.GIT_AUTHOR_EMAIL = process.env.GIT_COMMITTER_EMAIL = 'test@example.com'

const scratch = mkdtempSync(join(tmpdir(), 'lg-gitmove-'))
after(() => rmSync(scratch, { recursive: true, force: true }))
let repoCount = 0

const LINES = ['l1', 'l2', 'l3', 'l4', 'l5', 'l6', 'l7', 'l8']

function git(cwd: string, ...args: string[]): string {
  return execFileSync('git', args, { cwd, encoding: 'utf8' }).trim()
}

function write(root: string, name: string, lines: string[]): void {
  writeFileSync(join(root, name), lines.join('\n') + '\n')
}

function read(root: string, name: string): string[] {
  return readFileSync(join(root, name), 'utf8').trimEnd().split('\n')
}

// main and other share a.txt except that other changed its third line.
function repo(): string {
  const root = join(scratch, `repo${++repoCount}`)
  execFileSync('git', ['init', '-q', '-b', 'main', root])
  write(root, 'a.txt', LINES)
  git(root, 'add', '-A')
  git(root, 'commit', '-q', '-m', 'base')
  git(root, 'switch', '-q', '-c', 'other')
  write(root, 'a.txt', ['l1', 'l2', 'other3', ...LINES.slice(3)])
  git(root, 'commit', '-q', '-am', 'other')
  git(root, 'switch', '-q', 'main')
  return root
}

function runner(root: string): GitRunner {
  return {
    read: (args) => run(root, args, { env: readEnv }),
    write: (args) => run(root, args, { timeout: WRITE_TIMEOUT_MS })
  }
}

function branch(root: string): string {
  return git(root, 'symbolic-ref', '--short', 'HEAD')
}

function stashes(root: string): string[] {
  return git(root, 'stash', 'list', '--format=%gs').split('\n').filter(Boolean)
}

test('a clean tree switches with nothing to carry', async () => {
  const root = repo()
  const result = await moveBranch(runner(root), { kind: 'switch', name: 'other', remote: false })
  assert.deepEqual(result, { ok: true, from: 'main', to: 'other', carry: 'none' })
  assert.equal(branch(root), 'other')
})

test('unstaged and untracked changes are carried and the stash is dropped', async () => {
  const root = repo()
  write(root, 'a.txt', ['mine1', ...LINES.slice(1)])
  write(root, 'new.txt', ['fresh'])
  const result = await moveBranch(runner(root), { kind: 'switch', name: 'other', remote: false })
  assert.deepEqual(result, { ok: true, from: 'main', to: 'other', carry: 'carried' })
  assert.equal(branch(root), 'other')
  assert.deepEqual(read(root, 'a.txt'), ['mine1', 'l2', 'other3', ...LINES.slice(3)])
  assert.deepEqual(read(root, 'new.txt'), ['fresh'])
  assert.deepEqual(stashes(root), [])
})

test('a staged change stays staged on the target', async () => {
  const root = repo()
  write(root, 'b.txt', ['staged'])
  git(root, 'add', 'b.txt')
  const result = await moveBranch(runner(root), { kind: 'switch', name: 'other', remote: false })
  assert.equal(result.ok && result.carry, 'carried')
  assert.equal(git(root, 'diff', '--cached', '--name-only'), 'b.txt')
  assert.deepEqual(stashes(root), [])
})

test('creating from the current branch keeps the tree without a stash', async () => {
  const root = repo()
  write(root, 'a.txt', ['mine1', ...LINES.slice(1)])
  git(root, 'add', 'a.txt')
  const result = await moveBranch(runner(root), { kind: 'create', name: 'feat', base: 'main' })
  assert.deepEqual(result, { ok: true, from: 'main', to: 'feat', carry: 'none' })
  assert.equal(branch(root), 'feat')
  assert.equal(git(root, 'diff', '--cached', '--name-only'), 'a.txt')
  assert.deepEqual(stashes(root), [])
})

test('creating from a remote base carries the changes and sets no upstream', async () => {
  const origin = repo()
  const root = join(scratch, `clone${repoCount}`)
  execFileSync('git', ['clone', '-q', origin, root])
  write(root, 'new.txt', ['fresh'])
  const result = await moveBranch(runner(root), {
    kind: 'create',
    name: 'feat',
    base: 'origin/other'
  })
  assert.deepEqual(result, { ok: true, from: 'main', to: 'feat', carry: 'carried' })
  assert.equal(branch(root), 'feat')
  assert.deepEqual(read(root, 'a.txt')[2], 'other3')
  assert.deepEqual(read(root, 'new.txt'), ['fresh'])
  const upstream = await run(root, ['config', 'branch.feat.remote'])
  assert.notEqual(upstream.code, 0)
})

test('an invalid or taken name is refused before anything moves', async () => {
  const root = repo()
  const taken = await moveBranch(runner(root), { kind: 'create', name: 'other', base: 'main' })
  assert.equal(taken.ok, false)
  const invalid = await moveBranch(runner(root), { kind: 'create', name: 'a..b', base: 'main' })
  assert.equal(invalid.ok, false)
  const shorthand = await moveBranch(runner(root), { kind: 'create', name: '@{-1}', base: 'main' })
  assert.equal(shorthand.ok, false)
  assert.equal(branch(root), 'main')
})

test('a refused switch puts the changes back on the original branch', async () => {
  const root = repo()
  git(root, 'worktree', 'add', '-q', join(scratch, `wt${repoCount}`), 'other')
  write(root, 'a.txt', ['mine1', ...LINES.slice(1)])
  git(root, 'add', 'a.txt')
  write(root, 'new.txt', ['fresh'])
  const result = await moveBranch(runner(root), { kind: 'switch', name: 'other', remote: false })
  assert.equal(result.ok, false)
  assert.equal(branch(root), 'main')
  assert.equal(git(root, 'diff', '--cached', '--name-only'), 'a.txt')
  assert.deepEqual(read(root, 'new.txt'), ['fresh'])
  assert.deepEqual(stashes(root), [])
})

test('a conflicting apply keeps the stash', async () => {
  const root = repo()
  write(root, 'a.txt', ['l1', 'l2', 'mine3', ...LINES.slice(3)])
  const result = await moveBranch(runner(root), { kind: 'switch', name: 'other', remote: false })
  assert.equal(result.ok && result.carry, 'conflict')
  assert.equal(branch(root), 'other')
  assert.deepEqual(stashes(root), ['On main: lemongrass: carry main to other'])
})

test('a refused --index falls back to a plain apply and keeps the stash', async () => {
  const root = repo()
  // The staged first line's patch context covers the third line, which other changed, so the
  // index cannot be restored, while a three-way merge of the same change is clean.
  write(root, 'a.txt', ['mine1', ...LINES.slice(1)])
  git(root, 'add', 'a.txt')
  const result = await moveBranch(runner(root), { kind: 'switch', name: 'other', remote: false })
  assert.ok(result.ok)
  assert.equal(result.carry, 'carried')
  assert.match(result.notice ?? '', /staging was not restored/)
  assert.deepEqual(read(root, 'a.txt'), ['mine1', 'l2', 'other3', ...LINES.slice(3)])
  assert.deepEqual(stashes(root), ['On main: lemongrass: carry main to other'])
})
