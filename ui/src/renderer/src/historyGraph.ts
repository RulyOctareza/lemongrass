// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

export interface GraphLine {
  fromLane: number
  toLane: number
  // 0 is the top of the row, 0.5 its middle where the commit sits, 1 the bottom.
  fromY: 0 | 0.5
  toY: 0.5 | 1
  color: number
}

export interface GraphRow {
  lane: number
  color: number
  lines: GraphLine[]
}

export interface GraphInput {
  hash: string
  parents: string[]
}

export interface Graph {
  rows: GraphRow[]
  lanes: number
}

function freeLane(lanes: (string | null)[]): number {
  const free = lanes.indexOf(null)
  return free === -1 ? lanes.length : free
}

// Lays commits (newest first, parents after children) into lanes. A lane waits for the hash it
// expects next, and a commit takes the lane waiting for it, or a free one when none is.
export function buildGraph(commits: GraphInput[]): Graph {
  let lanes: (string | null)[] = []
  const rows: GraphRow[] = []
  let width = 0

  for (const commit of commits) {
    let lane = lanes.indexOf(commit.hash)
    const isTip = lane === -1
    if (isTip) {
      lane = freeLane(lanes)
      lanes[lane] = commit.hash
    }
    const before = lanes.slice()
    const after = lanes.slice()
    const lines: GraphLine[] = []

    for (let k = 0; k < before.length; k++) {
      if (before[k] === null || (isTip && k === lane)) continue
      if (before[k] === commit.hash) {
        lines.push({ fromLane: k, toLane: lane, fromY: 0, toY: 0.5, color: k })
        after[k] = null
      } else {
        lines.push({ fromLane: k, toLane: k, fromY: 0, toY: 1, color: k })
      }
    }

    commit.parents.forEach((parent, index) => {
      let target = after.indexOf(parent)
      if (target === -1) {
        target = index === 0 ? lane : freeLane(after)
        after[target] = parent
      }
      lines.push({ fromLane: lane, toLane: target, fromY: 0.5, toY: 1, color: target })
    })

    while (after.length > 0 && after[after.length - 1] === null) after.pop()
    width = Math.max(width, before.length, after.length, lane + 1)
    rows.push({ lane, color: lane, lines })
    lanes = after
  }
  return { rows, lanes: width }
}
