// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type {
  GitConflictChunk,
  GitConflictFile,
  GitConflictOperation,
  GitLineRange
} from '../../preload/types'
import type { MiddleProposal } from './conflictModel'

// Only the active project's workspace is mounted, so switching projects unmounts the conflict tab
// and every file it cached. Each file's work in progress is kept here by project and path and
// handed back when that file is mounted again.
export interface ConflictDraft {
  // The stages the draft was built from. A draft for other stages is stale.
  stages: string
  // The chunks the middle was loaded from, which Reset goes back to.
  chunks: GitConflictChunk[]
  finalNewline: boolean
  // The model's submission the chunks were mapped from, when the middle was a proposal.
  proposal?: MiddleProposal | null
  loadedText: string
  refs: ConflictRefs
  middle: unknown
  workingSeen: string | null
  focus: boolean
  sizes: number[]
  current: number | null
}

export interface ConflictRefs {
  operation: GitConflictOperation
  leftRef: string
  rightRef: string
}

// One file's partial work as it would be saved before a review. markSeen records the saved text
// as the working text already seen, so the save never brings up the reload prompt.
export interface ConflictWork {
  path: string
  text: string
  resolved: GitLineRange[]
  markSeen: (text: string) => void
}

const drafts = new Map<string, ConflictDraft>()
// Mounted resolver files, shown or cached by the conflict tab, each giving its work or null.
const live = new Map<string, () => Omit<ConflictWork, 'path'> | null>()

function keyOf(projectId: string, path: string): string {
  return `${projectId}\n${path}`
}

export function stagesOf(file: Extract<GitConflictFile, { status: 'ok' }>): string {
  return JSON.stringify([file.base, file.left, file.right])
}

export function saveDraft(projectId: string, path: string, draft: ConflictDraft): void {
  drafts.set(keyOf(projectId, path), draft)
}

// Hands a draft back once, and only for the stages it was built from.
export function takeDraft(projectId: string, path: string, stages: string): ConflictDraft | null {
  const key = keyOf(projectId, path)
  const draft = drafts.get(key)
  drafts.delete(key)
  return draft && draft.stages === stages ? draft : null
}

export function dropDraft(projectId: string, path: string): void {
  drafts.delete(keyOf(projectId, path))
}

export function registerWork(
  projectId: string,
  path: string,
  give: () => Omit<ConflictWork, 'path'> | null
): () => void {
  const key = keyOf(projectId, path)
  live.set(key, give)
  return () => {
    if (live.get(key) === give) live.delete(key)
  }
}

// The resolver files of a project with work: mounted ones, then drafts of files not mounted now.
export function workSourcesOf(projectId: string): {
  live: ConflictWork[]
  drafts: { path: string; draft: ConflictDraft }[]
} {
  const prefix = keyOf(projectId, '')
  const work: ConflictWork[] = []
  for (const [key, give] of live) {
    const found = key.startsWith(prefix) ? give() : null
    if (found) work.push({ path: key.slice(prefix.length), ...found })
  }
  const kept = [...drafts]
    .filter(([key]) => key.startsWith(prefix) && !live.has(key))
    .map(([key, draft]) => ({ path: key.slice(prefix.length), draft }))
  return { live: work, drafts: kept }
}
