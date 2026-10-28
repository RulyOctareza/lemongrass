// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { GitCarry } from '../preload/types'

export interface BranchNotice {
  from: string
  to: string
  carry: GitCarry
}

export interface BranchNoticeQueue {
  record(projectPath: string, notice: BranchNotice): void
  flush(): void
}

const CARRY_RANK: Record<GitCarry, number> = { none: 0, carried: 1, conflict: 2 }

// A burst of moves reads as one move from where it started to where it ended, with the worst
// carry seen on the way.
export function mergeNotices(first: BranchNotice, next: BranchNotice): BranchNotice {
  const carry = CARRY_RANK[next.carry] > CARRY_RANK[first.carry] ? next.carry : first.carry
  return { from: first.from, to: next.to, carry }
}

// Holds a project's branch notice until no move has happened for delayMs, so a wrong checkout
// followed by the right one reaches agent tabs as a single notice.
export function createNoticeQueue(
  send: (projectPath: string, notice: BranchNotice) => void,
  delayMs: number
): BranchNoticeQueue {
  const pending = new Map<string, { notice: BranchNotice; timer: ReturnType<typeof setTimeout> }>()

  function release(projectPath: string): void {
    const entry = pending.get(projectPath)
    if (!entry) return
    clearTimeout(entry.timer)
    pending.delete(projectPath)
    // A burst that ended where it started changed nothing the agents need to hear.
    if (entry.notice.from !== entry.notice.to) send(projectPath, entry.notice)
  }

  return {
    record(projectPath, notice) {
      const previous = pending.get(projectPath)
      if (previous) clearTimeout(previous.timer)
      pending.set(projectPath, {
        notice: previous ? mergeNotices(previous.notice, notice) : notice,
        timer: setTimeout(() => release(projectPath), delayMs)
      })
    },
    flush() {
      for (const projectPath of [...pending.keys()]) release(projectPath)
    }
  }
}
