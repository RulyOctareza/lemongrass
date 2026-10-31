// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { WorkgroupThreadMessage } from '../../preload/types'

// What the git panel says about one conflicted file once the model has worked on it.
export type ConflictFileStatus =
  { kind: 'proposed'; notes: number } | { kind: 'markers'; blocks: number }

// Adds newly read messages to the ones already held, by id, oldest first, so a poll that reads
// only the newest page never drops what was paged in before.
export function mergeMessages(
  held: WorkgroupThreadMessage[],
  incoming: WorkgroupThreadMessage[]
): WorkgroupThreadMessage[] {
  const byId = new Map(held.map((message) => [message.id, message]))
  for (const message of incoming) byId.set(message.id, message)
  return [...byId.values()].sort((a, b) => a.id - b.id)
}

// The model's latest submission. A later submit after a follow-up replaces every earlier one.
export function latestSubmit(messages: WorkgroupThreadMessage[]): WorkgroupThreadMessage | null {
  for (let i = messages.length - 1; i >= 0; i--) {
    if (messages[i].kind === 'submit') return messages[i]
  }
  return null
}

// The model's notes on one file, oldest first.
export function notesFor(
  messages: WorkgroupThreadMessage[],
  path: string
): WorkgroupThreadMessage[] {
  return messages.filter((message) => message.kind === 'note' && message.anchor?.path === path)
}

// The state of a file the model worked on, as the git panel and the resolver's file picker word it.
export function fileStatusText(status: ConflictFileStatus): string {
  if (status.kind === 'markers') return 'markers left'
  if (status.notes === 0) return 'proposed'
  return status.notes === 1 ? 'proposed, 1 note' : `proposed, ${status.notes} notes`
}

// A file the latest submission lists is proposed, unless it still holds marker blocks, which the
// model left for the human. A file it does not list has nothing from the model yet.
export function fileStatus(
  messages: WorkgroupThreadMessage[],
  path: string,
  conflictBlocks: number | null | undefined
): ConflictFileStatus | null {
  const submit = latestSubmit(messages)
  if (!submit || !submit.files.includes(path)) return null
  if (conflictBlocks) return { kind: 'markers', blocks: conflictBlocks }
  return { kind: 'proposed', notes: notesFor(messages, path).length }
}
