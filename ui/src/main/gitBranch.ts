// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import { execFile } from 'child_process'
import { WRITE_TIMEOUT_MS, errorText, readEnv, repoRoot, run, stoppedOperation } from './gitRun'
import { isValidNewBranch, moveBranch, type BranchMove, type GitRunner } from './gitMove'
import type { GitBranches, GitMoveResult } from '../preload/types'

const notARepo = 'This project is not a git repository.'
const LGRASSD_TIMEOUT_MS = 5_000

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

function runner(root: string): GitRunner {
  return {
    read: (args) => run(root, args, { env: readEnv }),
    write: (args) => run(root, args, { timeout: WRITE_TIMEOUT_MS })
  }
}

// The panel greys the switcher out too, but the tree can change between a poll and the click.
async function blockedReason(root: string): Promise<string | null> {
  const operation = await stoppedOperation(root)
  if (operation) return `A ${operation} is stopped. Continue or abort it before switching branches.`
  const conflicts = await run(root, ['diff', '--name-only', '--diff-filter=U'], { env: readEnv })
  if (conflicts.code !== 0) return errorText(conflicts)
  if (conflicts.stdout.trim()) return 'Resolve the conflicts before switching branches.'
  return null
}

// Tells the project's recently active agent tabs that the branch moved. The move has already
// happened, so a failure here is only logged.
function notifyTabs(lgrassdPath: string | null, projectPath: string, moved: GitMoveResult): void {
  if (!lgrassdPath || !moved.ok) return
  execFile(
    lgrassdPath,
    ['branch', 'changed', moved.from, moved.to, moved.carry],
    { cwd: projectPath, encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS },
    (failure, _stdout, stderr) => {
      if (failure) console.error('lgrassd branch changed failed:', stderr.trim() || failure)
    }
  )
}

async function move(
  lgrassdPath: string | null,
  projectPath: string,
  branchMove: BranchMove
): Promise<GitMoveResult> {
  const root = await repoRoot(projectPath)
  if (!root) return { ok: false, error: notARepo }
  const blocked = await blockedReason(root)
  if (blocked) return { ok: false, error: blocked }
  const moved = await moveBranch(runner(root), branchMove)
  notifyTabs(lgrassdPath, projectPath, moved)
  return moved
}

async function branchNameValid(projectPath: string, name: string): Promise<boolean> {
  const root = await repoRoot(projectPath)
  return !!root && isValidNewBranch(runner(root), name)
}

export function registerGitBranchHandlers(lgrassdPath: string | null): void {
  ipcMain.handle('git:branches', (_event, projectPath: string) => branches(projectPath))
  ipcMain.handle('git:switchBranch', (_event, projectPath: string, name: string, remote: boolean) =>
    move(lgrassdPath, projectPath, { kind: 'switch', name, remote: remote === true })
  )
  ipcMain.handle('git:createBranch', (_event, projectPath: string, name: string, base: string) =>
    move(lgrassdPath, projectPath, { kind: 'create', name, base })
  )
  ipcMain.handle('git:branchNameValid', (_event, projectPath: string, name: string) =>
    branchNameValid(projectPath, name)
  )
}
