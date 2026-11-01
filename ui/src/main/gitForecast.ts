// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { RunResult } from './gitRun'
import type { GitRunner } from './gitMove'
import type { GitForecast } from '../preload/types'

// Like gitMove.ts, this file has no value imports so node --test can load it straight from source.

// merge-tree --write-tree merges in memory: it adds blobs and trees to the object store, which gc
// prunes, and never touches the working tree, the index or a ref.

function errorText(result: RunResult): string {
  return result.stderr.trim() || result.stdout.trim() || 'git failed without a message.'
}

type MergeStep =
  | Exclude<GitForecast, { status: 'clean' } | { status: 'needsDependency' }>
  | { status: 'clean'; tree: string }

// With -z, --name-only and --no-messages the output is the tree id, then each conflicted path once,
// all NUL terminated. Exit 1 means conflicts, anything above it an error.
async function mergeTree(git: GitRunner, args: string[]): Promise<MergeStep> {
  const result = await git.read([
    'merge-tree',
    '--write-tree',
    '--name-only',
    '--no-messages',
    '-z',
    ...args
  ])
  if (result.code !== 0 && result.code !== 1)
    return { status: 'unavailable', error: errorText(result) }
  const [tree, ...paths] = result.stdout.split('\0').filter((part) => part !== '')
  if (!tree) return { status: 'unavailable', error: 'git merge-tree printed no tree.' }
  if (result.code === 0) return { status: 'clean', tree }
  return { status: 'conflict', paths, stop: null }
}

// The caller has checked that source is a branch.
export async function forecastMerge(git: GitRunner, source: string): Promise<GitForecast> {
  const result = await mergeTree(git, ['HEAD', source])
  return result.status === 'clean' ? { status: 'clean' } : result
}

// Replays what `git rebase <upstream> <name>` would replay, one merge-tree step per commit with the
// commit's parent as the base, carrying each result tree forward. --cherry-pick --right-only
// --no-merges picks the commits rebase itself picks: merges dropped, and commits whose change the
// upstream already has skipped. Only the first stop can be known, since later commits replay onto
// the human's resolution of it.
export async function forecastRebase(git: GitRunner, name: string): Promise<GitForecast> {
  if (!name || name.startsWith('-') || name.includes('\0')) {
    return { status: 'unavailable', error: 'That is not a local branch.' }
  }
  const upstream = await git.read(['for-each-ref', '--format=%(upstream)', `refs/heads/${name}`])
  const tracking = upstream.stdout.trim()
  if (upstream.code !== 0 || !tracking) {
    return { status: 'unavailable', error: `${name} has no upstream to rebase onto.` }
  }
  const listed = await git.read([
    'rev-list',
    '--reverse',
    '--no-merges',
    '--cherry-pick',
    '--right-only',
    '--parents',
    `${tracking}...refs/heads/${name}`
  ])
  if (listed.code !== 0) return { status: 'unavailable', error: errorText(listed) }
  const commits = listed.stdout
    .split('\n')
    .filter((line) => line !== '')
    .map((line) => line.split(' '))
  let tree = `${tracking}^{tree}`
  for (const [index, [commit, parent]] of commits.entries()) {
    if (!parent) return { status: 'unavailable', error: 'The branch replays a root commit.' }
    const step = await mergeTree(git, [`--merge-base=${parent}`, tree, `${commit}^{tree}`])
    if (step.status === 'unavailable') return step
    if (step.status === 'conflict') {
      const subject = await git.read(['show', '-s', '--format=%s', commit])
      return {
        ...step,
        stop: { index: index + 1, total: commits.length, subject: subject.stdout.trim() }
      }
    }
    tree = step.tree
  }
  return { status: 'clean' }
}
