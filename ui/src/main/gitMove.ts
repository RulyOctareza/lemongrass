// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import type { RunResult } from './gitRun'
import type { GitCarry, GitMoveResult } from '../preload/types'

// Bound to one repo by the caller. Kept as an interface so this file has no value imports and
// node --test can load it straight from source.
export interface GitRunner {
  read(args: string[]): Promise<RunResult>
  write(args: string[]): Promise<RunResult>
}

export type BranchMove =
  { kind: 'switch'; name: string; remote: boolean } | { kind: 'create'; name: string; base: string }

// A name that starts with a dash would be read by git as an option.
function isRefArgument(name: string): boolean {
  return name.length > 0 && !name.startsWith('-') && !name.includes('\0')
}

function errorText(result: RunResult): string {
  return result.stderr.trim() || result.stdout.trim() || 'git failed without a message.'
}

// The branch name, or the short commit hash when HEAD is detached.
export async function currentName(git: GitRunner): Promise<string> {
  const branch = await git.read(['symbolic-ref', '--short', '-q', 'HEAD'])
  if (branch.code === 0 && branch.stdout.trim()) return branch.stdout.trim()
  const hash = await git.read(['rev-parse', '--short', 'HEAD'])
  return hash.code === 0 && hash.stdout.trim() ? hash.stdout.trim() : 'HEAD'
}

// --branch also expands shorthands such as @{-1}, so the output must equal the input.
export async function isValidNewBranch(git: GitRunner, name: string): Promise<boolean> {
  if (!isRefArgument(name)) return false
  const result = await git.read(['check-ref-format', '--branch', name])
  return result.code === 0 && result.stdout.trim() === name
}

// The stash list can change while the move runs, so a stash is found by its commit hash, never
// by its stash@{N} position.
async function dropByHash(git: GitRunner, hash: string): Promise<void> {
  const list = await git.read(['stash', 'list', '--format=%gd%x1f%H'])
  const ref = list.stdout
    .split('\n')
    .map((line) => line.split('\x1f'))
    .find(([, lineHash]) => lineHash === hash)?.[0]
  if (ref) await git.write(['stash', 'drop', ref])
}

function switchArgs(move: BranchMove): string[] {
  if (move.kind === 'create') return ['switch', '--no-track', '-c', move.name, move.base]
  return move.remote ? ['switch', '--track', move.name] : ['switch', move.name]
}

function targetName(move: BranchMove): string {
  if (move.kind === 'switch' && move.remote) return move.name.slice(move.name.indexOf('/') + 1)
  return move.name
}

async function validate(git: GitRunner, move: BranchMove): Promise<string | null> {
  if (!isRefArgument(move.name)) return 'That is not a valid branch name.'
  if (move.kind === 'switch') return null
  if (!(await isValidNewBranch(git, move.name))) return 'That is not a valid branch name.'
  const exists = await git.read(['show-ref', '--verify', '--quiet', `refs/heads/${move.name}`])
  if (exists.code === 0) return `A branch named ${move.name} already exists.`
  if (!isRefArgument(move.base)) return 'That is not a valid base.'
  const base = await git.read(['rev-parse', '--verify', '--quiet', `${move.base}^{commit}`])
  if (base.code !== 0) return `${move.base} is not a commit to start from.`
  return null
}

// Applies the carry stash on the branch the move landed on and settles whether it is dropped.
async function carryOver(
  git: GitRunner,
  hash: string,
  from: string,
  to: string
): Promise<{ carry: GitCarry; notice?: string }> {
  const indexed = await git.write(['stash', 'apply', '--index', hash])
  if (indexed.code === 0) {
    await dropByHash(git, hash)
    return { carry: 'carried' }
  }
  const kept = `The stash "lemongrass: carry ${from} to ${to}" is kept in Stashes.`
  // git refuses --index before touching the tree when the staged part cannot be restored, and the
  // stash is then the only copy of what was staged, so it is kept even after a clean plain apply.
  if (!errorText(indexed).includes('Try without --index')) {
    return { carry: 'conflict', notice: `The carried changes conflicted with ${to}. ${kept}` }
  }
  const plain = await git.write(['stash', 'apply', hash])
  if (plain.code === 0) {
    return {
      carry: 'carried',
      notice: `The changes were carried, but staging was not restored. ${kept}`
    }
  }
  return { carry: 'conflict', notice: `The carried changes conflicted with ${to}. ${kept}` }
}

export async function moveBranch(git: GitRunner, move: BranchMove): Promise<GitMoveResult> {
  const from = await currentName(git)
  const invalid = await validate(git, move)
  if (invalid) return { ok: false, error: invalid }
  const to = targetName(move)

  // Creating at the current commit leaves HEAD where it is, so the tree comes along untouched.
  const headStays = move.kind === 'create' && (move.base === from || move.base === 'HEAD')
  const dirty = headStays
    ? null
    : await git.read(['status', '--porcelain', '--untracked-files=all'])
  if (dirty && dirty.code !== 0) return { ok: false, error: errorText(dirty) }

  if (!dirty || !dirty.stdout.trim()) {
    const switched = await git.write(switchArgs(move))
    if (switched.code !== 0) return { ok: false, error: errorText(switched) }
    return { ok: true, from, to, carry: 'none' }
  }

  const stashed = await git.write([
    'stash',
    'push',
    '--include-untracked',
    '-m',
    `lemongrass: carry ${from} to ${to}`
  ])
  if (stashed.code !== 0) return { ok: false, error: errorText(stashed) }
  // git exits 0 with nothing saved when the changes vanished since the status read, and
  // stash@{0} would then be an older stash.
  if (stashed.stdout.includes('No local changes to save')) {
    const switched = await git.write(switchArgs(move))
    if (switched.code !== 0) return { ok: false, error: errorText(switched) }
    return { ok: true, from, to, carry: 'none' }
  }
  const hash = (await git.read(['rev-parse', '--verify', '--quiet', 'stash@{0}'])).stdout.trim()
  if (!hash) return { ok: false, error: 'The changes were stashed, but the stash was not found.' }

  const switched = await git.write(switchArgs(move))
  if (switched.code !== 0) {
    const restored = await git.write(['stash', 'apply', '--index', hash])
    if (restored.code !== 0) {
      return {
        ok: false,
        error: `${errorText(switched)}\nThe changes could not be restored on ${from}. They are kept in Stashes.`
      }
    }
    await dropByHash(git, hash)
    return { ok: false, error: errorText(switched) }
  }

  return { ok: true, from, to, ...(await carryOver(git, hash, from, to)) }
}
