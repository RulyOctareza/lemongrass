// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// A matched span of the path, start inclusive, end exclusive.
export type MatchRange = [number, number]

export interface FuzzyMatch {
  score: number
  ranges: MatchRange[]
}

// A query that falls entirely inside the last segment outranks any match that needs the folders,
// so typing a file's name finds the file before every path that merely contains its letters.
const NAME_BONUS = 1000
const CONSECUTIVE_BONUS = 6
const BOUNDARY_BONUS = 8
const START_BONUS = 12
const GAP_PENALTY = 0.5
// Start positions tried per segment, which bounds the cost on long paths full of the first letter.
const MAX_STARTS = 24

function isBoundary(text: string, index: number): boolean {
  if (index === 0) return true
  const before = text[index - 1]
  if ('/._- '.includes(before)) return true
  const here = text[index]
  return before === before.toLowerCase() && here !== here.toLowerCase()
}

// Matches the query's characters in order inside text[from..], trying each start position for the
// first character and walking greedily from there, which keeps runs together in practice.
function matchFrom(
  text: string,
  lower: string,
  query: string,
  from: number
): { score: number; positions: number[] } | null {
  let best: { score: number; positions: number[] } | null = null
  let starts = 0
  for (let start = lower.indexOf(query[0], from); start !== -1;) {
    const positions = [start]
    let at = start + 1
    for (let q = 1; q < query.length; q++) {
      const found = lower.indexOf(query[q], at)
      if (found === -1) {
        positions.length = 0
        break
      }
      positions.push(found)
      at = found + 1
    }
    if (positions.length === 0) break
    let score = 0
    for (let i = 0; i < positions.length; i++) {
      const pos = positions[i]
      score += 1
      if (isBoundary(text, pos)) score += BOUNDARY_BONUS
      if (i > 0) {
        const gap = pos - positions[i - 1] - 1
        if (gap === 0) score += CONSECUTIVE_BONUS
        else score -= gap * GAP_PENALTY
      }
    }
    if (positions[0] === from) score += START_BONUS
    if (!best || score > best.score) best = { score, positions }
    if (++starts >= MAX_STARTS) break
    start = lower.indexOf(query[0], start + 1)
  }
  return best
}

function toRanges(positions: number[]): MatchRange[] {
  const ranges: MatchRange[] = []
  for (const pos of positions) {
    const last = ranges[ranges.length - 1]
    if (last && last[1] === pos) last[1] = pos + 1
    else ranges.push([pos, pos + 1])
  }
  return ranges
}

export function normalizeQuery(query: string): string {
  return query.replace(/\s+/g, '').toLowerCase()
}

// Scores a project-relative path against a query, case-insensitive with whitespace ignored. Returns
// null when the query's characters do not all appear in order. Higher scores rank first.
export function fuzzyMatch(path: string, rawQuery: string): FuzzyMatch | null {
  const query = normalizeQuery(rawQuery)
  if (!query) return null
  const lower = path.toLowerCase()
  const nameStart = path.lastIndexOf('/') + 1
  // A query with a slash is about the folders, so it never gets the name-only bonus.
  if (!query.includes('/')) {
    const inName = matchFrom(path, lower, query, nameStart)
    if (inName) {
      let score = NAME_BONUS + inName.score
      const name = lower.slice(nameStart)
      if (name === query || name.replace(/\.[^.]*$/, '') === query) score += START_BONUS * 4
      score -= path.length * 0.01
      return { score, ranges: toRanges(inName.positions) }
    }
  }
  const anywhere = matchFrom(path, lower, query, 0)
  if (!anywhere) return null
  return { score: anywhere.score - path.length * 0.01, ranges: toRanges(anywhere.positions) }
}

// The part of the ranges that falls inside [start, end), shifted to start at 0, for highlighting
// one segment of a path.
export function rangesWithin(ranges: MatchRange[], start: number, end: number): MatchRange[] {
  const out: MatchRange[] = []
  for (const [a, b] of ranges) {
    const from = Math.max(a, start)
    const to = Math.min(b, end)
    if (from < to) out.push([from - start, to - start])
  }
  return out
}

export interface SearchCandidate {
  path: string
  type: 'dir' | 'file'
}

export interface SearchNode {
  name: string
  path: string
  type: 'dir' | 'file'
  // Matched characters inside this node's own name, for highlighting.
  ranges: MatchRange[]
  // True for a path the query matched, false for a folder shown only to hold matches.
  match: boolean
  // The best score in this node's subtree, which orders siblings.
  score: number
  children: SearchNode[]
}

export interface SearchRow {
  node: SearchNode
  depth: number
}

export interface SearchResult {
  roots: SearchNode[]
  rows: SearchRow[]
  // Matches beyond the cap that were dropped.
  hidden: number
}

export const SEARCH_MATCH_CAP = 200

// Ranks candidates against the query and builds the smallest tree that holds the best matches,
// every folder open, siblings ordered by the best match below them. Both sidebar views render this
// so they rank and highlight the same way.
export function buildSearchTree(
  candidates: SearchCandidate[],
  query: string,
  cap = SEARCH_MATCH_CAP
): SearchResult {
  const scored: { candidate: SearchCandidate; score: number; ranges: MatchRange[] }[] = []
  for (const candidate of candidates) {
    const match = fuzzyMatch(candidate.path, query)
    if (match) scored.push({ candidate, score: match.score, ranges: match.ranges })
  }
  scored.sort((a, b) => b.score - a.score || a.candidate.path.localeCompare(b.candidate.path))
  const kept = scored.slice(0, cap)

  const byPath = new Map<string, SearchNode>()
  const roots: SearchNode[] = []
  for (const { candidate, score, ranges } of kept) {
    const segments = candidate.path.split('/')
    let parent: SearchNode | null = null
    let offset = 0
    for (let i = 0; i < segments.length; i++) {
      const name = segments[i]
      const path = segments.slice(0, i + 1).join('/')
      const last = i === segments.length - 1
      let node = byPath.get(path)
      if (!node) {
        node = {
          name,
          path,
          type: last ? candidate.type : 'dir',
          ranges: rangesWithin(ranges, offset, offset + name.length),
          match: false,
          score,
          children: []
        }
        byPath.set(path, node)
        if (parent) parent.children.push(node)
        else roots.push(node)
      }
      if (last) {
        node.match = true
        node.type = candidate.type
        node.ranges = rangesWithin(ranges, offset, offset + name.length)
      }
      node.score = Math.max(node.score, score)
      parent = node
      offset += name.length + 1
    }
  }

  const order = (nodes: SearchNode[]): void => {
    nodes.sort((a, b) => b.score - a.score || a.name.localeCompare(b.name))
    for (const node of nodes) order(node.children)
  }
  order(roots)

  const rows: SearchRow[] = []
  const walk = (nodes: SearchNode[], depth: number): void => {
    for (const node of nodes) {
      rows.push({ node, depth })
      walk(node.children, depth + 1)
    }
  }
  walk(roots, 0)

  return { roots, rows, hidden: scored.length - kept.length }
}
