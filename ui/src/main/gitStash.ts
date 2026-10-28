// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { WRITE_TIMEOUT_MS, errorText, readEnv, repoRoot, run } from './gitRun'
import type { GitActionResult, GitStash } from '../preload/types'

const notARepo = 'This project is not a git repository.'
const STASH_REF = /^stash@\{\d+\}$/

async function list(projectPath: string): Promise<GitStash[]> {
  const root = await repoRoot(projectPath)
  if (!root) return []
  const result = await run(root, ['stash', 'list', '--format=%gd%x1f%gs%x1f%ct'], { env: readEnv })
  if (result.code !== 0) return []
  return result.stdout
    .split('\n')
    .filter(Boolean)
    .map((line) => {
      const [ref, message, time] = line.split('\x1f')
      return { ref, message, time: Number(time) }
    })
}

async function push(
  projectPath: string,
  message: string,
  includeUntracked: boolean
): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const text = message.trim()
  const args = ['stash', 'push', ...(includeUntracked ? ['--include-untracked'] : [])]
  if (text) args.push('-m', text)
  const result = await run(root, args, { timeout: WRITE_TIMEOUT_MS })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  // With nothing to save git exits 0 and only says so.
  if (result.stdout.includes('No local changes to save')) {
    return { ok: false, error: 'There are no local changes to stash.' }
  }
  return { ok: true }
}

async function apply(
  projectPath: string,
  ref: string,
  action: 'apply' | 'pop' | 'drop'
): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  if (!STASH_REF.test(ref)) return { ok: false, error: 'That is not a valid stash.' }
  const result = await run(root, ['stash', action, ref], { timeout: WRITE_TIMEOUT_MS })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

export function registerGitStashHandlers(): void {
  ipcMain.handle('git:stashList', (_event, projectPath: string) => list(projectPath))
  ipcMain.handle(
    'git:stashPush',
    (_event, projectPath: string, message: string, includeUntracked: boolean) =>
      push(projectPath, message, includeUntracked)
  )
  ipcMain.handle(
    'git:stashAction',
    (_event, projectPath: string, ref: string, action: 'apply' | 'pop' | 'drop') => {
      if (action !== 'apply' && action !== 'pop' && action !== 'drop') {
        return { ok: false, error: 'That is not a stash action.' }
      }
      return apply(projectPath, ref, action)
    }
  )
}
