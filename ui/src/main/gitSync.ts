// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import {
  WRITE_TIMEOUT_MS,
  errorText,
  pushEnv,
  readEnv,
  repoRoot,
  run,
  stoppedOperation
} from './gitRun'
import type { GitActionResult } from '../preload/types'

const notARepo = 'This project is not a git repository.'
const FETCH_TIMEOUT_MS = 60_000
// An unset editor makes git wait on a terminal that is not there.
const noEditor = { GIT_EDITOR: 'true' }

// A target is a branch the repository already has, so nothing else can reach git as a revision.
async function isBranch(root: string, name: string): Promise<boolean> {
  if (!name || name.startsWith('-') || name.includes('\0')) return false
  for (const prefix of ['refs/heads/', 'refs/remotes/']) {
    const found = await run(root, ['show-ref', '--verify', '--quiet', `${prefix}${name}`], {
      env: readEnv
    })
    if (found.code === 0) return true
  }
  return false
}

async function fetchAll(projectPath: string): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  // --prune drops remote branches deleted on the remote. git never moves a tag it already has, so
  // a tag deleted and recreated on the remote needs --tags --force to follow it. --prune-tags is
  // left out because it would also delete local tags that were never pushed.
  const result = await run(root, ['fetch', '--all', '--prune', '--tags', '--force'], {
    timeout: FETCH_TIMEOUT_MS,
    env: pushEnv
  })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

// How many commits merging the source would bring in.
async function mergeCount(projectPath: string, source: string): Promise<number> {
  const root = await repoRoot(projectPath)
  if (!root || !(await isBranch(root, source))) return 0
  const result = await run(root, ['rev-list', '--count', `HEAD..${source}`], { env: readEnv })
  return result.code === 0 ? Number(result.stdout.trim()) : 0
}

// A conflict stops the merge for the human, and the panel then offers Continue and Abort. There is
// no rebase here, since rebasing a pushed branch would need the force push this app never does.
async function merge(projectPath: string, source: string): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  if (!(await isBranch(root, source))) return { ok: false, error: 'That is not a branch.' }
  if (await stoppedOperation(root)) {
    return { ok: false, error: 'A merge or rebase is already in progress.' }
  }
  const result = await run(root, ['merge', '--no-edit', source], {
    timeout: WRITE_TIMEOUT_MS,
    env: noEditor
  })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

async function finish(projectPath: string, action: 'continue' | 'abort'): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const operation = await stoppedOperation(root)
  if (!operation) return { ok: false, error: 'No merge or rebase is in progress.' }
  const result = await run(root, [operation, `--${action}`], {
    timeout: WRITE_TIMEOUT_MS,
    env: noEditor
  })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

export function registerGitSyncHandlers(): void {
  ipcMain.handle('git:fetch', (_event, projectPath: string) => fetchAll(projectPath))
  ipcMain.handle('git:mergeCount', (_event, projectPath: string, source: string) =>
    mergeCount(projectPath, source)
  )
  ipcMain.handle('git:merge', (_event, projectPath: string, source: string) =>
    merge(projectPath, source)
  )
  ipcMain.handle('git:finish', (_event, projectPath: string, action: 'continue' | 'abort') => {
    if (action !== 'continue' && action !== 'abort') return { ok: false, error: 'Unknown action.' }
    return finish(projectPath, action)
  })
}
