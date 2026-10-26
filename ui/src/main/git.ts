// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import { registerGitBranchHandlers } from './gitBranch'
import { registerGitHistoryHandlers } from './gitHistory'
import { registerGitRevisionHandlers } from './gitRevision'
import { registerGitSyncHandlers } from './gitSync'
import { registerGitStashHandlers } from './gitStash'
import { parseDiff, parseStatus } from './gitParse'
import {
  WRITE_TIMEOUT_MS,
  errorText,
  isInside,
  pushEnv,
  readEnv,
  repoRoot,
  run,
  stoppedOperation,
  topLevelCache,
  type RunResult
} from './gitRun'
import type { GitActionResult, GitDiff, GitStatus } from '../preload/types'

const EMPTY_TREE = '4b825dc642cb6eb9a060e54bf8d69288fbee4904'
const PUSH_TIMEOUT_MS = 60_000
const MAX_DIFF_BYTES = 4 * 1024 * 1024

const notARepo: GitStatus = {
  isRepo: false,
  branch: null,
  detached: false,
  upstream: null,
  ahead: 0,
  behind: 0,
  hasCommits: false,
  pushTarget: null,
  operation: null,
  files: []
}

async function pushRemote(root: string): Promise<string | null> {
  const result = await run(root, ['remote'], { env: readEnv })
  const remotes = result.stdout.split('\n').filter(Boolean)
  if (remotes.length === 0) return null
  return remotes.includes('origin') ? 'origin' : remotes[0]
}

async function status(projectPath: string): Promise<GitStatus> {
  const root = await repoRoot(projectPath)
  if (!root) return notARepo
  const result = await run(
    root,
    ['status', '--porcelain=v2', '--branch', '-z', '--untracked-files=all'],
    { env: readEnv }
  )
  if (result.code !== 0) {
    topLevelCache.delete(projectPath)
    return notARepo
  }
  const parsed = parseStatus(result.stdout)
  let pushTarget = parsed.upstream
  if (!pushTarget && parsed.branch) {
    const remote = await pushRemote(root)
    pushTarget = remote ? `${remote}/${parsed.branch}` : null
  }
  return { isRepo: true, ...parsed, pushTarget, operation: await stoppedOperation(root) }
}

async function isUntracked(root: string, path: string): Promise<boolean> {
  const result = await run(root, ['ls-files', '--others', '--exclude-standard', '-z', '--', path], {
    env: readEnv
  })
  return result.code === 0 && result.stdout.split('\0').includes(path)
}

async function diff(projectPath: string, path: string, origPath?: string): Promise<GitDiff> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'error', message: 'This project is not a git repository.' }
  if (!isInside(root, path) || (origPath !== undefined && !isInside(root, origPath))) {
    return { status: 'error', message: 'That path is outside the repository.' }
  }

  const untracked = await isUntracked(root, path)
  let result: RunResult
  if (untracked) {
    result = await run(
      root,
      ['diff', '--no-index', '--no-color', '--no-ext-diff', '-U3', '--', '/dev/null', path],
      { env: readEnv, maxBuffer: MAX_DIFF_BYTES }
    )
  } else {
    const head = await run(root, ['rev-parse', '--verify', '--quiet', 'HEAD'], { env: readEnv })
    const base = head.code === 0 ? 'HEAD' : EMPTY_TREE
    const paths = origPath ? [path, origPath] : [path]
    result = await run(
      root,
      ['diff', base, '--no-color', '--no-ext-diff', '--no-textconv', '-M', '-U3', '--', ...paths],
      { env: readEnv, maxBuffer: MAX_DIFF_BYTES }
    )
  }
  if (result.overflow) return { status: 'too-large' }
  // A no-index diff exits with 1 when the files differ.
  if (result.code !== 0 && !(untracked && result.code === 1)) {
    return { status: 'error', message: errorText(result) }
  }
  return parseDiff(result.stdout)
}

async function commit(
  projectPath: string,
  message: string,
  paths?: string[]
): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: 'This project is not a git repository.' }
  const text = message.trim()
  if (!text) return { ok: false, error: 'The commit message is empty.' }
  if (paths && (paths.length === 0 || !paths.every((path) => isInside(root, path)))) {
    return { ok: false, error: 'The selected files are not valid paths in this repository.' }
  }

  // With paths, only untracked files need adding. The path-limited commit reads every tracked
  // path from the worktree and leaves all other files, staged or not, untouched. Adding tracked
  // paths would fail for a staged rename or deletion, whose old path is in neither index nor worktree.
  const env = { GIT_LITERAL_PATHSPECS: '1' }
  let toAdd = ['-A']
  if (paths) {
    const listed = await run(
      root,
      ['ls-files', '--others', '--exclude-standard', '-z', '--', ...paths],
      { env }
    )
    if (listed.code !== 0) return { ok: false, error: errorText(listed) }
    const untracked = listed.stdout.split('\0').filter(Boolean)
    toAdd = untracked.length > 0 ? ['-A', '--', ...untracked] : []
  }
  if (toAdd.length > 0) {
    const add = await run(root, ['add', ...toAdd], { timeout: WRITE_TIMEOUT_MS, env })
    if (add.code !== 0) return { ok: false, error: errorText(add) }
  }
  const result = await run(root, ['commit', '-F', '-', ...(paths ? ['--', ...paths] : [])], {
    input: `${text}\n`,
    timeout: WRITE_TIMEOUT_MS,
    env
  })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

async function push(projectPath: string): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: 'This project is not a git repository.' }

  const branchResult = await run(root, ['symbolic-ref', '--short', '-q', 'HEAD'], {
    env: readEnv
  })
  const branch = branchResult.stdout.trim()
  if (branchResult.code !== 0 || !branch) {
    return { ok: false, error: 'HEAD is detached, so there is no branch to push.' }
  }

  const upstream = await run(root, ['rev-parse', '--abbrev-ref', '--symbolic-full-name', '@{u}'], {
    env: readEnv
  })
  let args: string[]
  if (upstream.code === 0) {
    args = ['push']
  } else {
    const remote = await pushRemote(root)
    if (!remote) return { ok: false, error: 'This repository has no remote to push to.' }
    args = ['push', '-u', remote, 'HEAD']
  }
  const result = await run(root, args, { timeout: PUSH_TIMEOUT_MS, env: pushEnv })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

async function lastMessage(projectPath: string): Promise<string> {
  const root = await repoRoot(projectPath)
  if (!root) return ''
  const result = await run(root, ['log', '-1', '--format=%B'], { env: readEnv })
  return result.code === 0 ? result.stdout.trim() : ''
}

export function registerGitHandlers(lgrassdPath: string | null): void {
  registerGitBranchHandlers(lgrassdPath)
  registerGitRevisionHandlers()
  registerGitStashHandlers()
  registerGitSyncHandlers()
  registerGitHistoryHandlers()
  ipcMain.handle('git:status', (_event, projectPath: string) => status(projectPath))
  ipcMain.handle('git:diff', (_event, projectPath: string, path: string, origPath?: string) =>
    diff(projectPath, path, origPath)
  )
  ipcMain.handle('git:commit', (_event, projectPath: string, message: string, paths?: string[]) =>
    commit(projectPath, message, paths)
  )
  ipcMain.handle('git:push', (_event, projectPath: string) => push(projectPath))
  ipcMain.handle('git:lastMessage', (_event, projectPath: string) => lastMessage(projectPath))
}
