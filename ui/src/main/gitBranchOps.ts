// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { RunResult } from './gitRun'
import type { GitRunner } from './gitMove'
import type { GitDeleteResult, GitMoveResult, GitUpdateResult } from '../preload/types'

// Like gitMove.ts, this file has no value imports so node --test can load it straight from source.

// Deleting the checked-out branch moves HEAD here first, so this branch itself is never deleted.
export const FALLBACK_BRANCH = 'main'

function errorText(result: RunResult): string {
  return result.stderr.trim() || result.stdout.trim() || 'git failed without a message.'
}

// A name that starts with a dash would be read by git as an option.
function isRefArgument(name: string): boolean {
  return name.length > 0 && !name.startsWith('-') && !name.includes('\0')
}

async function localExists(git: GitRunner, name: string): Promise<boolean> {
  if (!isRefArgument(name)) return false
  return (await git.read(['show-ref', '--verify', '--quiet', `refs/heads/${name}`])).code === 0
}

async function currentBranch(git: GitRunner): Promise<string | null> {
  const result = await git.read(['symbolic-ref', '--short', '-q', 'HEAD'])
  return (result.code === 0 && result.stdout.trim()) || null
}

interface Upstream {
  // '.' when the upstream is another local branch, which has nothing to fetch.
  remote: string
  // The branch's name on the remote, such as refs/heads/main.
  remoteRef: string
  // The local ref that mirrors it, such as refs/remotes/origin/main.
  tracking: string
  short: string
}

async function upstreamOf(git: GitRunner, name: string): Promise<Upstream | null> {
  const result = await git.read([
    'for-each-ref',
    '--format=%(upstream:remotename)%1f%(upstream:remoteref)%1f%(upstream)%1f%(upstream:short)',
    `refs/heads/${name}`
  ])
  const [remote, remoteRef, tracking, short] = result.stdout.trim().split('\x1f')
  if (result.code !== 0 || !remote || !tracking) return null
  return { remote, remoteRef, tracking, short }
}

async function counts(
  git: GitRunner,
  name: string,
  tracking: string
): Promise<{ ahead: number; behind: number } | string> {
  const result = await git.read([
    'rev-list',
    '--left-right',
    '--count',
    `refs/heads/${name}...${tracking}`
  ])
  if (result.code !== 0) return errorText(result)
  const [ahead, behind] = result.stdout.trim().split(/\s+/).map(Number)
  return { ahead, behind }
}

// git reports a branch the remote no longer has this way when it is fetched by name.
function goneOnRemote(result: RunResult): boolean {
  return result.stderr.includes("couldn't find remote ref")
}

// Drops the remote-tracking ref of a branch the remote deleted, as fetch --prune would.
async function dropTracking(git: GitRunner, tracking: string): Promise<string | null> {
  const dropped = await git.write(['update-ref', '-d', tracking])
  return dropped.code === 0 ? null : errorText(dropped)
}

// Fetches the branch's upstream and fast-forwards the branch to it. A branch with commits of its
// own is reported as diverged and left untouched.
export async function updateLocal(git: GitRunner, name: string): Promise<GitUpdateResult> {
  if (!(await localExists(git, name)))
    return { status: 'error', error: 'That is not a local branch.' }
  const upstream = await upstreamOf(git, name)
  if (!upstream) return { status: 'error', error: `${name} has no upstream to update from.` }
  if (upstream.remote !== '.') {
    const fetched = await git.write(['fetch', upstream.remote, upstream.remoteRef])
    if (goneOnRemote(fetched)) {
      const failed = await dropTracking(git, upstream.tracking)
      if (failed) return { status: 'error', error: failed }
      return {
        status: 'gone',
        message: `${upstream.short} was deleted on ${upstream.remote}, so ${name} has no upstream now.`
      }
    }
    if (fetched.code !== 0) return { status: 'error', error: errorText(fetched) }
  }
  const sides = await counts(git, name, upstream.tracking)
  if (typeof sides === 'string') return { status: 'error', error: sides }
  if (sides.behind === 0) return { status: 'updated' }
  if (sides.ahead > 0) {
    return { status: 'diverged', branch: name, upstream: upstream.short, ...sides }
  }
  // Fetching from the repo itself moves a branch that is not checked out, and git refuses it for
  // a branch checked out in another worktree or a move that is not a fast-forward.
  const forwarded =
    name === (await currentBranch(git))
      ? await git.write(['merge', '--ff-only', upstream.tracking])
      : await git.write(['fetch', '.', `${upstream.tracking}:refs/heads/${name}`])
  if (forwarded.code !== 0) return { status: 'error', error: errorText(forwarded) }
  return { status: 'updated' }
}

// Remote names may contain slashes, so the longest remote that prefixes the name wins.
export async function updateRemote(git: GitRunner, remoteBranch: string): Promise<GitUpdateResult> {
  const remotes = (await git.read(['remote'])).stdout.split('\n').filter(Boolean)
  const remote = remotes
    .filter((name) => remoteBranch.startsWith(`${name}/`))
    .sort((a, b) => b.length - a.length)[0]
  const branch = remote ? remoteBranch.slice(remote.length + 1) : ''
  if (!remote || !isRefArgument(branch)) {
    return { status: 'error', error: 'That is not a remote branch.' }
  }
  const fetched = await git.write(['fetch', remote, `refs/heads/${branch}`])
  if (goneOnRemote(fetched)) {
    // The default fetch refspec keeps a remote's branches under refs/remotes/<remote>/.
    const failed = await dropTracking(git, `refs/remotes/${remoteBranch}`)
    if (failed) return { status: 'error', error: failed }
    return {
      status: 'gone',
      message: `${remoteBranch} was deleted on ${remote}, so it is removed from the list.`
    }
  }
  if (fetched.code !== 0) return { status: 'error', error: errorText(fetched) }
  return { status: 'updated' }
}

// Replays the branch's own commits on top of its upstream. A branch that is not checked out is
// checked out first, since git rebases only the current branch.
export async function rebaseOntoUpstream(
  git: GitRunner,
  name: string
): Promise<{ ok: true } | { ok: false; error: string }> {
  if (!(await localExists(git, name))) return { ok: false, error: 'That is not a local branch.' }
  const upstream = await upstreamOf(git, name)
  if (!upstream) return { ok: false, error: `${name} has no upstream to rebase onto.` }
  const current = await currentBranch(git)
  const args =
    name === current ? ['rebase', upstream.tracking] : ['rebase', upstream.tracking, name]
  const result = await git.write(args)
  // A stop on conflicts exits non-zero too, and the panel then offers Continue and Abort.
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

function commitsExist(count: number): string {
  return count === 1 ? '1 commit exists' : `${count} commits exist`
}

// The branch's commits that no other local branch (merged) and no remote branch (pushed) has, so
// deleting the branch would lose them for good. Returns git's error text when it cannot tell.
export async function commitsOnlyOn(git: GitRunner, name: string): Promise<number | string> {
  if (!(await localExists(git, name))) return 'That is not a local branch.'
  const result = await git.read([
    'rev-list',
    '--count',
    `refs/heads/${name}`,
    '--not',
    `--exclude=${name}`,
    '--branches',
    '--remotes'
  ])
  if (result.code !== 0) return errorText(result)
  return Number(result.stdout.trim())
}

// Deleting the checked-out branch moves to main first through moveToFallback, which carries
// uncommitted changes the way a checkout does. Any other branch is deleted without touching the
// tree.
export async function deleteBranch(
  git: GitRunner,
  name: string,
  moveToFallback: () => Promise<GitMoveResult>
): Promise<GitDeleteResult> {
  if (!(await localExists(git, name))) {
    return { ok: false, error: 'That is not a local branch.', moved: null }
  }
  const isCurrent = name === (await currentBranch(git))
  if (isCurrent && name === FALLBACK_BRANCH) {
    return {
      ok: false,
      error: `Deleting a branch falls back to ${FALLBACK_BRANCH}. Check out another branch to delete ${FALLBACK_BRANCH}.`,
      moved: null
    }
  }
  if (isCurrent && !(await localExists(git, FALLBACK_BRANCH))) {
    return {
      ok: false,
      error: `There is no ${FALLBACK_BRANCH} branch to fall back to.`,
      moved: null
    }
  }
  const only = await commitsOnlyOn(git, name)
  if (typeof only === 'string') return { ok: false, error: only, moved: null }
  if (only > 0) {
    return {
      ok: false,
      error: `${commitsExist(only)} only on ${name}. Merge or push them before deleting it.`,
      moved: null
    }
  }
  let moved: GitDeleteResult['moved'] = null
  if (isCurrent) {
    const result = await moveToFallback()
    if (!result.ok) return { ok: false, error: result.error, moved: null }
    const { from, to, carry, notice } = result
    moved = notice ? { from, to, carry, notice } : { from, to, carry }
  }
  // The check above replaces git's own merged check, which only looks at HEAD and the upstream and
  // so refuses a branch that is merged elsewhere or only pushed.
  const deleted = await git.write(['branch', '-D', name])
  if (deleted.code !== 0) return { ok: false, error: errorText(deleted), moved }
  return { ok: true, moved }
}
