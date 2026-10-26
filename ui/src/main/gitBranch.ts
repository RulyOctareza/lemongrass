// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import { WRITE_TIMEOUT_MS, errorText, readEnv, repoRoot, run } from './gitRun'
import type { GitActionResult, GitBranches } from '../preload/types'

const notARepo = 'This project is not a git repository.'

// A name that starts with a dash would be read by git as an option.
function isBranchName(name: string): boolean {
  return name.length > 0 && !name.startsWith('-') && !name.includes('\0')
}

async function branches(projectPath: string): Promise<GitBranches> {
  const root = await repoRoot(projectPath)
  const empty: GitBranches = { current: null, local: [], remote: [] }
  if (!root) return empty
  const refs = await run(
    root,
    ['for-each-ref', '--format=%(refname)', '--sort=refname', 'refs/heads', 'refs/remotes'],
    { env: readEnv }
  )
  if (refs.code !== 0) return empty
  const current = await run(root, ['symbolic-ref', '--short', '-q', 'HEAD'], { env: readEnv })
  const result: GitBranches = { current: current.stdout.trim() || null, local: [], remote: [] }
  for (const ref of refs.stdout.split('\n').filter(Boolean)) {
    if (ref.startsWith('refs/heads/')) result.local.push(ref.slice('refs/heads/'.length))
    // origin/HEAD only points at another remote branch.
    else if (ref.startsWith('refs/remotes/') && !ref.endsWith('/HEAD')) {
      result.remote.push(ref.slice('refs/remotes/'.length))
    }
  }
  return result
}

async function switchBranch(
  projectPath: string,
  name: string,
  remote: boolean
): Promise<GitActionResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  if (!isBranchName(name)) return { ok: false, error: 'That is not a valid branch name.' }

  // The panel greys the switcher out too, but the index can change between a poll and the click.
  const staged = await run(root, ['diff', '--cached', '--quiet'], { env: readEnv })
  if (staged.code === 1) {
    return {
      ok: false,
      error: 'There are staged changes. Commit or unstage them before switching.'
    }
  }
  if (staged.code !== 0) return { ok: false, error: errorText(staged) }

  const args = remote ? ['switch', '--track', name] : ['switch', name]
  const result = await run(root, args, { timeout: WRITE_TIMEOUT_MS })
  if (result.code !== 0) return { ok: false, error: errorText(result) }
  return { ok: true }
}

export function registerGitBranchHandlers(): void {
  ipcMain.handle('git:branches', (_event, projectPath: string) => branches(projectPath))
  ipcMain.handle('git:switchBranch', (_event, projectPath: string, name: string, remote: boolean) =>
    switchBranch(projectPath, name, remote)
  )
}
