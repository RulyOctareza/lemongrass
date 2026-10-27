// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import { parseDiff, parseNameStatus } from './gitParse'
import { errorText, isInside, readEnv, repoRoot, run } from './gitRun'
import type { GitDiff, GitRevFile, GitRevFiles, GitRevision } from '../preload/types'

const EMPTY_TREE = '4b825dc642cb6eb9a060e54bf8d69288fbee4904'
const MAX_DIFF_BYTES = 4 * 1024 * 1024
const COMMIT_HASH = /^[0-9a-f]{4,64}$/
const STASH_REF = /^stash@\{\d+\}$/

interface Range {
  base: string
  tip: string
  // A stash made with --include-untracked keeps those files as the third parent's tree.
  untracked: string | null
}

// The renderer names a revision, never a range, so nothing it sends reaches git as an option.
async function resolve(root: string, rev: GitRevision): Promise<Range | null> {
  if (rev.kind === 'commit') {
    if (!COMMIT_HASH.test(rev.hash)) return null
    const parent = await run(root, ['rev-parse', '--verify', '--quiet', `${rev.hash}^1`], {
      env: readEnv
    })
    return {
      base: parent.code === 0 ? `${rev.hash}^1` : EMPTY_TREE,
      tip: rev.hash,
      untracked: null
    }
  }
  if (!STASH_REF.test(rev.ref)) return null
  const third = await run(root, ['rev-parse', '--verify', '--quiet', `${rev.ref}^3`], {
    env: readEnv
  })
  return { base: `${rev.ref}^1`, tip: rev.ref, untracked: third.code === 0 ? `${rev.ref}^3` : null }
}

async function untrackedPaths(root: string, range: Range): Promise<string[]> {
  if (!range.untracked) return []
  const listed = await run(root, ['ls-tree', '-r', '--name-only', '-z', range.untracked], {
    env: readEnv
  })
  return listed.code === 0 ? listed.stdout.split('\0').filter(Boolean) : []
}

async function revFiles(projectPath: string, rev: GitRevision): Promise<GitRevFiles> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'error', message: 'This project is not a git repository.' }
  const range = await resolve(root, rev)
  if (!range) return { status: 'error', message: 'That is not a valid revision.' }
  const result = await run(
    root,
    ['diff', '--name-status', '-z', '-M', '--no-color', '--no-ext-diff', range.base, range.tip],
    { env: readEnv }
  )
  if (result.code !== 0) return { status: 'error', message: errorText(result) }
  const files: GitRevFile[] = parseNameStatus(result.stdout)
  for (const path of await untrackedPaths(root, range)) files.push({ path, kind: 'added' })
  return { status: 'ok', files: files.sort((a, b) => a.path.localeCompare(b.path)) }
}

async function revDiff(
  projectPath: string,
  rev: GitRevision,
  path: string,
  origPath?: string
): Promise<GitDiff> {
  const root = await repoRoot(projectPath)
  if (!root) return { status: 'error', message: 'This project is not a git repository.' }
  if (!isInside(root, path) || (origPath !== undefined && !isInside(root, origPath))) {
    return { status: 'error', message: 'That path is outside the repository.' }
  }
  const range = await resolve(root, rev)
  if (!range) return { status: 'error', message: 'That is not a valid revision.' }

  const fromUntracked = (await untrackedPaths(root, range)).includes(path)
  const [base, tip] =
    fromUntracked && range.untracked ? [EMPTY_TREE, range.untracked] : [range.base, range.tip]
  const paths = origPath ? [path, origPath] : [path]
  const result = await run(
    root,
    [
      'diff',
      base,
      tip,
      '--no-color',
      '--no-ext-diff',
      '--no-textconv',
      '-M',
      '-U3',
      '--',
      ...paths
    ],
    { env: readEnv, maxBuffer: MAX_DIFF_BYTES }
  )
  if (result.overflow) return { status: 'too-large' }
  if (result.code !== 0) return { status: 'error', message: errorText(result) }
  return parseDiff(result.stdout)
}

export function registerGitRevisionHandlers(): void {
  ipcMain.handle('git:revFiles', (_event, projectPath: string, rev: GitRevision) =>
    revFiles(projectPath, rev)
  )
  ipcMain.handle(
    'git:revDiff',
    (_event, projectPath: string, rev: GitRevision, path: string, origPath?: string) =>
      revDiff(projectPath, rev, path, origPath)
  )
}
