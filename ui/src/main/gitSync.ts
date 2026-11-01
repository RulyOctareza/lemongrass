// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import {
  WRITE_TIMEOUT_MS,
  errorText,
  isInside,
  pushEnv,
  readEnv,
  repoRoot,
  run,
  stoppedOperation
} from './gitRun'
import { applyResolution, pickSide, readConflict, saveWorking } from './gitConflict'
import { forecastMerge } from './gitForecast'
import type { GitRunner } from './gitMove'
import { endConflictThreadIfOver } from './conflictAgent'
import { noteGitFailure } from './notifications'
import type {
  GitActionResult,
  GitConflictChoice,
  GitConflictFile,
  GitForecast
} from '../preload/types'

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

async function mergeForecast(projectPath: string, source: string): Promise<GitForecast> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'unavailable', error: notARepo }
  if (!(await isBranch(root, source)))
    return { status: 'unavailable', error: 'That is not a branch.' }
  return forecastMerge(conflictRunner(root), source)
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

// Writes take paths, so pathspec magic such as :(glob) must stay off for them too.
function conflictRunner(root: string): GitRunner {
  return {
    read: (args) => run(root, args, { env: readEnv }),
    write: (args) =>
      run(root, args, { timeout: WRITE_TIMEOUT_MS, env: { GIT_LITERAL_PATHSPECS: '1' } })
  }
}

async function conflictRoot(
  projectPath: string,
  path: string
): Promise<string | { error: string }> {
  const root = await repoRoot(projectPath)
  if (!root) return { error: notARepo }
  if (!isInside(root, path)) return { error: 'That path is outside the repository.' }
  return root
}

async function conflict(projectPath: string, path: string): Promise<GitConflictFile> {
  const root = await conflictRoot(projectPath, path)
  if (typeof root !== 'string') return { status: 'error', ...root }
  return readConflict(conflictRunner(root), root, path)
}

async function conflictApply(
  projectPath: string,
  path: string,
  content: string
): Promise<GitActionResult> {
  const root = await conflictRoot(projectPath, path)
  if (typeof root !== 'string') return { ok: false, ...root }
  if (typeof content !== 'string') return { ok: false, error: 'No content to write.' }
  return applyResolution(conflictRunner(root), root, path, content)
}

async function conflictSave(
  projectPath: string,
  path: string,
  content: string
): Promise<GitActionResult> {
  const root = await conflictRoot(projectPath, path)
  if (typeof root !== 'string') return { ok: false, ...root }
  if (typeof content !== 'string') return { ok: false, error: 'No content to write.' }
  return saveWorking(conflictRunner(root), root, path, content)
}

async function conflictPick(
  projectPath: string,
  path: string,
  choice: GitConflictChoice
): Promise<GitActionResult> {
  const root = await conflictRoot(projectPath, path)
  if (typeof root !== 'string') return { ok: false, ...root }
  if (choice !== 'left' && choice !== 'right' && choice !== 'delete') {
    return { ok: false, error: 'Unknown choice.' }
  }
  return pickSide(conflictRunner(root), path, choice)
}

export function registerGitSyncHandlers(): void {
  ipcMain.handle('git:fetch', async (_event, projectPath: string) =>
    noteGitFailure(projectPath, 'fetch', await fetchAll(projectPath))
  )
  ipcMain.handle('git:mergeCount', (_event, projectPath: string, source: string) =>
    mergeCount(projectPath, source)
  )
  ipcMain.handle('git:mergeForecast', (_event, projectPath: string, source: string) =>
    mergeForecast(projectPath, source)
  )
  ipcMain.handle('git:merge', (_event, projectPath: string, source: string) =>
    merge(projectPath, source)
  )
  // Continue, abort, apply and pick are the git calls that can end a conflict, so each one lets the
  // conflict thread close when its scope is over.
  ipcMain.handle(
    'git:finish',
    async (_event, projectPath: string, action: 'continue' | 'abort') => {
      if (action !== 'continue' && action !== 'abort')
        return { ok: false, error: 'Unknown action.' }
      const result = await finish(projectPath, action)
      endConflictThreadIfOver(projectPath)
      return result
    }
  )
  ipcMain.handle('git:conflict', (_event, projectPath: string, path: string) =>
    conflict(projectPath, path)
  )
  ipcMain.handle(
    'git:conflictApply',
    async (_event, projectPath: string, path: string, content: string) => {
      const result = await conflictApply(projectPath, path, content)
      endConflictThreadIfOver(projectPath)
      return result
    }
  )
  ipcMain.handle('git:conflictSave', (_event, projectPath: string, path: string, content: string) =>
    conflictSave(projectPath, path, content)
  )
  ipcMain.handle(
    'git:conflictPick',
    async (_event, projectPath: string, path: string, choice: GitConflictChoice) => {
      const result = await conflictPick(projectPath, path, choice)
      endConflictThreadIfOver(projectPath)
      return result
    }
  )
}
