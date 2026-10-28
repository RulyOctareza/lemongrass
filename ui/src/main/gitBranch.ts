// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import {
  WRITE_TIMEOUT_MS,
  errorText,
  pushEnv,
  readEnv,
  repoRoot,
  run,
  stoppedOperation
} from './gitRun'
import {
  currentName,
  isValidNewBranch,
  moveBranch,
  type BranchMove,
  type GitRunner
} from './gitMove'
import {
  FALLBACK_BRANCH,
  commitsOnlyOn,
  deleteBranch,
  rebaseOntoUpstream,
  updateLocal,
  updateRemote
} from './gitBranchOps'
import { createNoticeQueue, type BranchNotice, type BranchNoticeQueue } from './gitNotice'
import { parseTrack } from './gitParse'
import type {
  GitActionResult,
  GitBranches,
  GitDeleteResult,
  GitMoveResult,
  GitUpdateResult
} from '../preload/types'

const notARepo = 'This project is not a git repository.'
const LGRASSD_TIMEOUT_MS = 5_000
const NOTICE_DELAY_MS = 5_000

async function branches(projectPath: string): Promise<GitBranches> {
  const root = await repoRoot(projectPath)
  const empty: GitBranches = { current: null, local: [], remote: [] }
  if (!root) return empty
  const refs = await run(
    root,
    [
      'for-each-ref',
      '--format=%(refname)%1f%(upstream:short)%1f%(upstream:track)',
      '--sort=refname',
      'refs/heads',
      'refs/remotes'
    ],
    { env: readEnv }
  )
  if (refs.code !== 0) return empty
  const current = await run(root, ['symbolic-ref', '--short', '-q', 'HEAD'], { env: readEnv })
  const result: GitBranches = { current: current.stdout.trim() || null, local: [], remote: [] }
  for (const line of refs.stdout.split('\n').filter(Boolean)) {
    const [ref, upstream, track] = line.split('\x1f')
    if (ref.startsWith('refs/heads/')) {
      // A gone upstream has nothing left to update from, so it counts as none.
      const gone = track === '[gone]'
      result.local.push({
        name: ref.slice('refs/heads/'.length),
        upstream: upstream && !gone ? upstream : null,
        ...parseTrack(track)
      })
    }
    // origin/HEAD only points at another remote branch.
    else if (ref.startsWith('refs/remotes/') && !ref.endsWith('/HEAD')) {
      result.remote.push(ref.slice('refs/remotes/'.length))
    }
  }
  return result
}

// Fetches inside updates must not wait on a credential prompt, and a rebase must not wait on an
// editor that is not there.
function runner(root: string): GitRunner {
  return {
    read: (args) => run(root, args, { env: readEnv }),
    write: (args) =>
      run(root, args, { timeout: WRITE_TIMEOUT_MS, env: { ...pushEnv, GIT_EDITOR: 'true' } })
  }
}

// The panel greys these actions out too, but the tree can change between a poll and the click.
async function blockedReason(root: string, doing: string): Promise<string | null> {
  const operation = await stoppedOperation(root)
  if (operation) return `A ${operation} is stopped. Continue or abort it before ${doing}.`
  const conflicts = await run(root, ['diff', '--name-only', '--diff-filter=U'], { env: readEnv })
  if (conflicts.code !== 0) return errorText(conflicts)
  if (conflicts.stdout.trim()) return `Resolve the conflicts before ${doing}.`
  return null
}

let notices: BranchNoticeQueue | null = null

// Tells the project's recently active agent tabs that the branch moved, once the human has stopped
// switching. The move has already happened, so a failure here is only logged.
function noticeQueue(lgrassdPath: string | null): BranchNoticeQueue {
  return createNoticeQueue((projectPath: string, notice: BranchNotice) => {
    if (!lgrassdPath) return
    execFile(
      lgrassdPath,
      ['branch', 'changed', notice.from, notice.to, notice.carry],
      { cwd: projectPath, encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS },
      (failure, _stdout, stderr) => {
        if (failure) console.error('lgrassd branch changed failed:', stderr.trim() || failure)
      }
    )
  }, NOTICE_DELAY_MS)
}

// Sends any notice still waiting out its delay, for when the app quits.
export function flushBranchNotices(): void {
  notices?.flush()
}

async function move(projectPath: string, branchMove: BranchMove): Promise<GitMoveResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const blocked = await blockedReason(root, 'switching branches')
  if (blocked) return { ok: false, error: blocked }
  const moved = await moveBranch(runner(root), branchMove)
  if (moved.ok) notices?.record(projectPath, { from: moved.from, to: moved.to, carry: moved.carry })
  return moved
}

async function update(
  projectPath: string,
  name: string,
  remote: boolean
): Promise<GitUpdateResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'error', error: notARepo }
  return remote ? updateRemote(runner(root), name) : updateLocal(runner(root), name)
}

async function rebase(projectPath: string, name: string): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const blocked = await blockedReason(root, 'rebasing')
  if (blocked) return { ok: false, error: blocked }
  const git = runner(root)
  const from = await currentName(git)
  const result = await rebaseOntoUpstream(git, name)
  // Rebasing another branch checks it out, even when git then stops on conflicts.
  if (from !== name && (result.ok || (await stoppedOperation(root)) === 'rebase')) {
    notices?.record(projectPath, { from, to: name, carry: 'none' })
  }
  return result
}

// Only a stopped merge or rebase blocks every delete. Conflicts matter only to the switch to main
// that deleting the checked-out branch makes, and that switch checks them itself.
async function remove(projectPath: string, name: string): Promise<GitDeleteResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo, moved: null }
  const operation = await stoppedOperation(root)
  if (operation) {
    return {
      ok: false,
      error: `A ${operation} is stopped. Continue or abort it before deleting a branch.`,
      moved: null
    }
  }
  const git = runner(root)
  const result = await deleteBranch(git, name, async () => {
    const blocked = await blockedReason(root, 'switching branches')
    if (blocked) return { ok: false, error: blocked }
    return moveBranch(git, { kind: 'switch', name: FALLBACK_BRANCH, remote: false })
  })
  if (result.moved) {
    const { from, to, carry } = result.moved
    notices?.record(projectPath, { from, to, carry })
  }
  return result
}

// null when git cannot tell, and the delete itself then reports why.
async function onlyOn(projectPath: string, name: string): Promise<number | null> {
  const root = await repoRoot(projectPath)
  if (!root) return null
  const count = await commitsOnlyOn(runner(root), name)
  return typeof count === 'number' ? count : null
}

async function branchNameValid(projectPath: string, name: string): Promise<boolean> {
  const root = await repoRoot(projectPath)
  return !!root && isValidNewBranch(runner(root), name)
}

export function registerGitBranchHandlers(lgrassdPath: string | null): void {
  notices = noticeQueue(lgrassdPath)
  ipcMain.handle('git:branches', (_event, projectPath: string) => branches(projectPath))
  ipcMain.handle('git:switchBranch', (_event, projectPath: string, name: string, remote: boolean) =>
    move(projectPath, { kind: 'switch', name, remote: remote === true })
  )
  ipcMain.handle('git:createBranch', (_event, projectPath: string, name: string, base: string) =>
    move(projectPath, { kind: 'create', name, base })
  )
  ipcMain.handle('git:branchNameValid', (_event, projectPath: string, name: string) =>
    branchNameValid(projectPath, name)
  )
  ipcMain.handle('git:updateBranch', (_event, projectPath: string, name: string, remote: boolean) =>
    update(projectPath, name, remote === true)
  )
  ipcMain.handle('git:rebaseOntoUpstream', (_event, projectPath: string, name: string) =>
    rebase(projectPath, name)
  )
  ipcMain.handle('git:deleteBranch', (_event, projectPath: string, name: string) =>
    remove(projectPath, name)
  )
  ipcMain.handle('git:commitsOnlyOn', (_event, projectPath: string, name: string) =>
    onlyOn(projectPath, name)
  )
}
