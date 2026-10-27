// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { reactive } from 'vue'
import type {
  GitFile,
  GitFileGroup,
  GitMoveResult,
  GitStash,
  GitStatus,
  GitUpdateResult
} from '../../preload/types'
import type { ProjectRef } from './workspace'

export type GitDiverged = Extract<GitUpdateResult, { status: 'diverged' }>

export interface GitState {
  status: GitStatus | null
  stashes: GitStash[]
  revision: number
  folded: boolean
  message: string
  messageSeeded: boolean
  unchecked: string[]
  foldedGroups: GitFileGroup[]
  busy: 'commit' | 'push' | 'stash' | 'fetch' | 'branch' | null
  // The branch the merge confirm is open for.
  mergeSource: string | null
  // A branch and its upstream that both moved, for the dialog that offers the rebase.
  diverged: GitDiverged | null
  error: string | null
}

const states = reactive<Record<string, GitState>>({})
const refreshing = new Set<string>()

export function gitStateOf(projectId: string): GitState {
  if (!states[projectId]) {
    states[projectId] = {
      status: null,
      stashes: [],
      revision: 0,
      folded: true,
      message: '',
      messageSeeded: false,
      unchecked: [],
      foldedGroups: [],
      busy: null,
      mergeSource: null,
      diverged: null,
      error: null
    }
  }
  return states[projectId]
}

export async function refreshGit(project: ProjectRef): Promise<void> {
  if (refreshing.has(project.id)) return
  refreshing.add(project.id)
  try {
    const next = await window.api.git.status(project.path)
    const state = gitStateOf(project.id)
    if (JSON.stringify(state.status) !== JSON.stringify(next)) state.status = next
    // A file that left the change list starts checked again if it changes later.
    const present = new Set(next.files.map((file) => file.path))
    if (state.unchecked.some((path) => !present.has(path))) {
      state.unchecked = state.unchecked.filter((path) => present.has(path))
    }
    const stashes = await window.api.git.stashList(project.path)
    if (JSON.stringify(state.stashes) !== JSON.stringify(stashes)) state.stashes = stashes
    state.revision += 1
    await seedMessage(project, state)
  } finally {
    refreshing.delete(project.id)
  }
}

// Fills an empty draft with the last commit message once per project, so the next commit starts
// from it. A draft the human cleared afterwards stays cleared.
async function seedMessage(project: ProjectRef, state: GitState): Promise<void> {
  if (state.messageSeeded || !state.status?.isRepo || !state.status.hasCommits) return
  state.messageSeeded = true
  if (state.message) return
  const last = await window.api.git.lastMessage(project.path)
  if (!state.message) state.message = last
}

export function isChecked(projectId: string, path: string): boolean {
  return !gitStateOf(projectId).unchecked.includes(path)
}

export function toggleFile(projectId: string, path: string): void {
  const state = gitStateOf(projectId)
  state.unchecked = isChecked(projectId, path)
    ? [...state.unchecked, path]
    : state.unchecked.filter((entry) => entry !== path)
}

export function toggleGroupFold(projectId: string, group: GitFileGroup): void {
  const state = gitStateOf(projectId)
  state.foldedGroups = state.foldedGroups.includes(group)
    ? state.foldedGroups.filter((entry) => entry !== group)
    : [...state.foldedGroups, group]
}

export function setAllChecked(projectId: string, files: GitFile[], checked: boolean): void {
  gitStateOf(projectId).unchecked = checked ? [] : files.map((file) => file.path)
}

export async function commitChanges(project: ProjectRef, andPush: boolean): Promise<void> {
  const state = gitStateOf(project.id)
  if (state.busy) return
  state.error = null
  state.busy = 'commit'
  const files = state.status?.files ?? []
  const selected = files.filter((file) => !state.unchecked.includes(file.path))
  // A rename must carry its old path too, or the deletion half stays behind.
  const paths =
    selected.length < files.length
      ? selected.flatMap((file) => (file.origPath ? [file.path, file.origPath] : [file.path]))
      : undefined
  const committed = await window.api.git.commit(project.path, state.message, paths)
  if (!committed.ok) {
    state.error = committed.error
    state.busy = null
    await refreshGit(project)
    return
  }
  if (andPush) {
    state.busy = 'push'
    const pushed = await window.api.git.push(project.path)
    if (!pushed.ok && !(pushed.diverged && (await offerRebase(project)))) {
      state.error = `The commit was created, but the push failed. ${pushed.error}`
    }
  }
  state.busy = null
  await refreshGit(project)
}

// Main carries uncommitted changes across the move. Returns git's error text, or null once the
// branch is checked out, and a carry that needs the human, such as a kept stash, shows as the
// panel's error.
async function moveBranch(
  project: ProjectRef,
  call: () => Promise<GitMoveResult>
): Promise<string | null> {
  const state = gitStateOf(project.id)
  if (state.busy) return 'Another git operation is running.'
  state.busy = 'branch'
  const moved = await call()
  state.busy = null
  if (moved.ok) state.error = moved.notice ?? null
  await refreshGit(project)
  return moved.ok ? null : moved.error
}

export function switchBranch(
  project: ProjectRef,
  name: string,
  remote: boolean
): Promise<string | null> {
  return moveBranch(project, () => window.api.git.switchBranch(project.path, name, remote))
}

export function createBranch(
  project: ProjectRef,
  name: string,
  base: string
): Promise<string | null> {
  return moveBranch(project, () => window.api.git.createBranch(project.path, name, base))
}

// Runs one stash action and refreshes. Returns git's error text, or null on success.
export async function runStash(
  project: ProjectRef,
  action: 'apply' | 'pop' | 'drop' | { push: string; includeUntracked: boolean },
  ref = ''
): Promise<string | null> {
  const state = gitStateOf(project.id)
  if (state.busy) return 'Another git operation is running.'
  state.busy = 'stash'
  const done =
    typeof action === 'string'
      ? await window.api.git.stashAction(project.path, ref, action)
      : await window.api.git.stashPush(project.path, action.push, action.includeUntracked)
  state.busy = null
  await refreshGit(project)
  return done.ok ? null : done.error
}

// The origin refused the push because it has commits the branch lacks. Updating fetches them and
// opens the diverged dialog, whose rebase is the way to push again. Returns whether it opened.
async function offerRebase(project: ProjectRef): Promise<boolean> {
  const state = gitStateOf(project.id)
  const branch = state.status?.branch
  if (!branch) return false
  const updated = await window.api.git.updateBranch(project.path, branch, false)
  if (updated.status !== 'diverged') return false
  state.diverged = updated
  return true
}

// Runs one action that may move or rewrite branches and refreshes. Returns git's error text, or
// null on success.
async function runBranchAction(
  project: ProjectRef,
  busy: 'fetch' | 'branch',
  call: () => Promise<{ ok: true } | { ok: false; error: string }>
): Promise<string | null> {
  const state = gitStateOf(project.id)
  if (state.busy) return 'Another git operation is running.'
  state.busy = busy
  const done = await call()
  state.busy = null
  await refreshGit(project)
  return done.ok ? null : done.error
}

export function fetchAll(project: ProjectRef): Promise<string | null> {
  return runBranchAction(project, 'fetch', () => window.api.git.fetch(project.path))
}

export function finishOperation(
  project: ProjectRef,
  action: 'continue' | 'abort'
): Promise<string | null> {
  return runBranchAction(project, 'branch', () => window.api.git.finish(project.path, action))
}

export function mergeInto(project: ProjectRef, source: string): Promise<string | null> {
  return runBranchAction(project, 'branch', () => window.api.git.merge(project.path, source))
}

export function rebaseOntoUpstream(project: ProjectRef, name: string): Promise<string | null> {
  return runBranchAction(project, 'branch', () =>
    window.api.git.rebaseOntoUpstream(project.path, name)
  )
}

// Deleting the checked-out branch carries uncommitted changes to main, and a carry that needs the
// human, such as a kept stash, shows as the panel's error.
export function deleteBranch(project: ProjectRef, name: string): Promise<string | null> {
  return runBranchAction(project, 'branch', async () => {
    const deleted = await window.api.git.deleteBranch(project.path, name)
    if (deleted.moved?.notice) gitStateOf(project.id).error = deleted.moved.notice
    return deleted
  })
}

// A diverged branch opens the dialog that offers the rebase instead of reporting an error.
export function updateBranch(
  project: ProjectRef,
  name: string,
  remote: boolean
): Promise<string | null> {
  return runBranchAction(project, 'branch', async () => {
    const updated = await window.api.git.updateBranch(project.path, name, remote)
    if (updated.status === 'diverged') gitStateOf(project.id).diverged = updated
    return updated.status === 'error' ? { ok: false, error: updated.error } : { ok: true }
  })
}

// Pushes the checked-out branch on its own, for commits made earlier. Returns git's error text, or
// null on success or when the diverged dialog took over.
export async function pushBranch(project: ProjectRef): Promise<string | null> {
  const state = gitStateOf(project.id)
  if (state.busy) return 'Another git operation is running.'
  state.busy = 'push'
  const pushed = await window.api.git.push(project.path)
  const error = pushed.ok || (pushed.diverged && (await offerRebase(project))) ? null : pushed.error
  state.busy = null
  await refreshGit(project)
  return error
}
