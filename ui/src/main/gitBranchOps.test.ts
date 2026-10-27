// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { WRITE_TIMEOUT_MS, readEnv, run } from './gitRun.ts'
import { moveBranch, type GitRunner } from './gitMove.ts'
import { deleteBranch, rebaseOntoUpstream, updateLocal, updateRemote } from './gitBranchOps.ts'

// The machine's own git config, such as commit signing or hooks, must not reach these repos.
process.env.GIT_CONFIG_GLOBAL = '/dev/null'
process.env.GIT_CONFIG_NOSYSTEM = '1'
process.env.GIT_AUTHOR_NAME = process.env.GIT_COMMITTER_NAME = 'Test'
process.env.GIT_AUTHOR_EMAIL = process.env.GIT_COMMITTER_EMAIL = 'test@example.com'

const scratch = mkdtempSync(join(tmpdir(), 'lg-gitops-'))
after(() => rmSync(scratch, { recursive: true, force: true }))
let setupCount = 0

function git(cwd: string, ...args: string[]): string {
  return execFileSync('git', args, { cwd, encoding: 'utf8' }).trim()
}

function commitFile(root: string, name: string, text: string): void {
  writeFileSync(join(root, name), `${text}\n`)
  git(root, 'add', '-A')
  git(root, 'commit', '-q', '-m', text)
}

// origin holds main and feat. local is the repo under test and tracks both; other is a second
// clone that moves origin ahead.
function setup(): { local: string; other: string } {
  const base = join(scratch, `set${++setupCount}`)
  const origin = join(base, 'origin.git')
  execFileSync('git', ['init', '-q', '--bare', '-b', 'main', origin])
  const other = join(base, 'other')
  // Cloning the still empty origin warns on stderr, which only clutters the test output.
  execFileSync('git', ['clone', '-q', origin, other], { stdio: 'ignore' })
  git(other, 'switch', '-q', '-c', 'main')
  commitFile(other, 'a.txt', 'base')
  git(other, 'switch', '-q', '-c', 'feat')
  commitFile(other, 'f.txt', 'feat')
  git(other, 'push', '-q', 'origin', 'main', 'feat')
  git(other, 'switch', '-q', 'main')
  const local = join(base, 'local')
  execFileSync('git', ['clone', '-q', origin, local])
  git(local, 'branch', '-q', '--track', 'feat', 'origin/feat')
  return { local, other }
}

function runner(root: string): GitRunner {
  return {
    read: (args) => run(root, args, { env: readEnv }),
    write: (args) => run(root, args, { timeout: WRITE_TIMEOUT_MS, env: { GIT_EDITOR: 'true' } })
  }
}

function branch(root: string): string {
  return git(root, 'symbolic-ref', '--short', 'HEAD')
}

function advance(other: string, name: string, file: string): void {
  git(other, 'switch', '-q', name)
  commitFile(other, file, `${name} ${file}`)
  git(other, 'push', '-q', 'origin', name)
}

test('a branch that is not checked out is fast-forwarded to its fetched upstream', async () => {
  const { local, other } = setup()
  advance(other, 'feat', 'g.txt')
  const result = await updateLocal(runner(local), 'feat')
  assert.deepEqual(result, { status: 'updated' })
  assert.equal(git(local, 'rev-parse', 'feat'), git(other, 'rev-parse', 'feat'))
  assert.equal(branch(local), 'main')
})

test('the checked-out branch is fast-forwarded in place', async () => {
  const { local, other } = setup()
  advance(other, 'main', 'm.txt')
  const result = await updateLocal(runner(local), 'main')
  assert.deepEqual(result, { status: 'updated' })
  assert.equal(git(local, 'rev-parse', 'HEAD'), git(other, 'rev-parse', 'main'))
})

test('a diverged branch is reported with both counts and left as it was', async () => {
  const { local, other } = setup()
  advance(other, 'main', 'm.txt')
  commitFile(local, 'mine.txt', 'mine')
  const before = git(local, 'rev-parse', 'HEAD')
  const result = await updateLocal(runner(local), 'main')
  assert.deepEqual(result, {
    status: 'diverged',
    branch: 'main',
    upstream: 'origin/main',
    ahead: 1,
    behind: 1
  })
  assert.equal(git(local, 'rev-parse', 'HEAD'), before)
})

test('a branch without an upstream cannot be updated', async () => {
  const { local } = setup()
  git(local, 'branch', '-q', 'solo')
  const result = await updateLocal(runner(local), 'solo')
  assert.equal(result.status, 'error')
})

test('a remote branch is fetched into its remote-tracking ref', async () => {
  const { local, other } = setup()
  advance(other, 'feat', 'g.txt')
  const result = await updateRemote(runner(local), 'origin/feat')
  assert.deepEqual(result, { status: 'updated' })
  assert.equal(git(local, 'rev-parse', 'origin/feat'), git(other, 'rev-parse', 'feat'))
})

test('rebasing the checked-out branch puts its own commits on top of the upstream', async () => {
  const { local, other } = setup()
  advance(other, 'main', 'm.txt')
  commitFile(local, 'mine.txt', 'mine')
  await updateLocal(runner(local), 'main')
  const result = await rebaseOntoUpstream(runner(local), 'main')
  assert.deepEqual(result, { ok: true })
  assert.equal(git(local, 'rev-parse', 'HEAD~1'), git(other, 'rev-parse', 'main'))
})

test('rebasing another branch checks it out first', async () => {
  const { local, other } = setup()
  advance(other, 'feat', 'g.txt')
  git(local, 'switch', '-q', 'feat')
  commitFile(local, 'mine.txt', 'mine')
  git(local, 'switch', '-q', 'main')
  await updateLocal(runner(local), 'feat')
  const result = await rebaseOntoUpstream(runner(local), 'feat')
  assert.deepEqual(result, { ok: true })
  assert.equal(branch(local), 'feat')
  assert.equal(git(local, 'rev-parse', 'HEAD~1'), git(other, 'rev-parse', 'feat'))
})

function toMain(root: string): () => ReturnType<typeof moveBranch> {
  return () => moveBranch(runner(root), { kind: 'switch', name: 'main', remote: false })
}

test('deleting the checked-out branch falls back to main', async () => {
  const { local } = setup()
  git(local, 'switch', '-q', '-c', 'temp')
  const result = await deleteBranch(runner(local), 'temp', toMain(local))
  assert.deepEqual(result, { ok: true, moved: { from: 'temp', to: 'main', carry: 'none' } })
  assert.equal(branch(local), 'main')
  assert.equal(git(local, 'branch', '--list', 'temp'), '')
})

test('uncommitted changes come along when the checked-out branch is deleted', async () => {
  const { local } = setup()
  git(local, 'switch', '-q', '-c', 'temp')
  writeFileSync(join(local, 'loose.txt'), 'loose\n')
  const result = await deleteBranch(runner(local), 'temp', toMain(local))
  assert.equal(result.ok, true)
  assert.equal(result.moved?.carry, 'carried')
  assert.equal(branch(local), 'main')
  assert.equal(readFileSync(join(local, 'loose.txt'), 'utf8'), 'loose\n')
})

test('another branch is deleted with uncommitted changes left in place', async () => {
  const { local } = setup()
  git(local, 'branch', '-q', 'temp')
  writeFileSync(join(local, 'loose.txt'), 'loose\n')
  const result = await deleteBranch(runner(local), 'temp', toMain(local))
  assert.deepEqual(result, { ok: true, moved: null })
  assert.equal(readFileSync(join(local, 'loose.txt'), 'utf8'), 'loose\n')
})

test('main is deleted when it matches its origin and is not checked out', async () => {
  const { local } = setup()
  git(local, 'switch', '-q', 'feat')
  const result = await deleteBranch(runner(local), 'main', toMain(local))
  assert.deepEqual(result, { ok: true, moved: null })
  assert.equal(git(local, 'branch', '--list', 'main'), '')
})

test('main cannot be deleted while it is checked out', async () => {
  const { local } = setup()
  const result = await deleteBranch(runner(local), 'main', toMain(local))
  assert.equal(result.ok, false)
  assert.equal(git(local, 'branch', '--list', 'main'), '* main')
})

test('a branch whose commits are merged into another branch or pushed is deleted', async () => {
  const { local } = setup()
  git(local, 'switch', '-q', '-c', 'temp')
  commitFile(local, 'temp.txt', 'temp')
  git(local, 'switch', '-q', 'feat')
  git(local, 'merge', '-q', '--no-edit', 'temp')
  git(local, 'switch', '-q', 'main')
  const merged = await deleteBranch(runner(local), 'temp', toMain(local))
  assert.deepEqual(merged, { ok: true, moved: null })

  git(local, 'switch', '-q', 'feat')
  git(local, 'push', '-q', 'origin', 'feat')
  git(local, 'switch', '-q', 'main')
  const pushed = await deleteBranch(runner(local), 'feat', toMain(local))
  assert.deepEqual(pushed, { ok: true, moved: null })
})

test('a branch with commits found nowhere else is kept', async () => {
  const { local } = setup()
  git(local, 'switch', '-q', '-c', 'temp')
  commitFile(local, 'unmerged.txt', 'unmerged')
  const result = await deleteBranch(runner(local), 'temp', toMain(local))
  assert.equal(result.ok, false)
  assert.equal(result.ok === false && result.error.startsWith('1 commit exists only on temp'), true)
  assert.equal(branch(local), 'temp')
})
