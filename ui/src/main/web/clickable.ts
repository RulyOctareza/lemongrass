// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import type { Cdp } from './cdp'

interface DocumentSnapshot {
  nodes: {
    parentIndex?: number[]
    backendNodeId?: number[]
    nodeType?: number[]
    nodeName?: number[]
  }
  // Bounds are x, y, width, height per layout entry; CDP always sends them.
  layout: { nodeIndex: number[]; styles: number[][]; bounds?: number[][] }
}

export interface DomSnapshot {
  documents: DocumentSnapshot[]
  strings: string[]
}

const ELEMENT_NODE = 1
// A page full of pointer cursors, such as a long table of clickable rows, would bury everything else.
const MAX_CLICKABLE = 60
// The order of the computed styles asked for in findClickable, which is the order of each layout style entry.
const STYLES = ['cursor', 'position', 'opacity', 'visibility']
const ROWS = new Set(['LI', 'TR'])

// Elements that set the pointer cursor themselves: the cursor is the page's own sign that a click does
// something here. An element whose cursor is only inherited from a pointer ancestor is left out, so a clickable
// card gives one target and not one per word inside it. A list that sets the pointer for its rows gives the
// rows instead, since each row is its own choice, as in a searchable select, unless the rows hold links or
// buttons of their own.
//
// `actionable` holds the elements that already get a ref from their role, such as links: a long sidebar would
// otherwise use up the cap. Elements inside a fixed or absolute layer come first, because that is where an
// open dialog or a dropdown appended to the end of body renders. An element a person cannot see, with no size,
// hidden, or under a transparent ancestor such as a closed panel waiting to fade in, is no target, and neither is
// one in `hidden`, which the page hides from people with aria-hidden or inert.
export function pointerOwners(
  snapshot: DomSnapshot,
  actionable = new Set<number>(),
  hidden = new Set<number>()
): Set<number> {
  const floating: number[] = []
  const rest: number[] = []
  for (const doc of snapshot.documents) {
    const { parentIndex = [], backendNodeId = [], nodeType = [], nodeName = [] } = doc.nodes
    const style = (i: number, name: string): string =>
      snapshot.strings[doc.layout.styles[i]?.[STYLES.indexOf(name)] ?? -1] ?? ''
    const cursor = new Map<number, string>()
    const positioned = new Set<number>()
    const unseen = new Set<number>()
    const transparent = new Set<number>()
    doc.layout.nodeIndex.forEach((node, i) => {
      cursor.set(node, style(i, 'cursor'))
      const position = style(i, 'position')
      if (position === 'fixed' || position === 'absolute') positioned.add(node)
      const box = doc.layout.bounds?.[i]
      if ((box && (box[2] <= 0 || box[3] <= 0)) || style(i, 'visibility') === 'hidden')
        unseen.add(node)
      if (style(i, 'opacity') === '0') transparent.add(node)
    })
    const children = new Map<number, number[]>()
    parentIndex.forEach((parent, node) => {
      if (parent < 0) return
      const list = children.get(parent)
      if (list) list.push(node)
      else children.set(parent, [node])
    })
    const inherited = (node: number): string => {
      for (let at = parentIndex[node]; at !== undefined && at >= 0; at = parentIndex[at]) {
        const found = cursor.get(at)
        if (found !== undefined) return found
      }
      return ''
    }
    const holdsActionable = new Set<number>()
    backendNodeId.forEach((id, node) => {
      if (!actionable.has(id)) return
      for (let at = parentIndex[node]; at !== undefined && at >= 0; at = parentIndex[at])
        holdsActionable.add(at)
    })
    const inLayer = (node: number): boolean => {
      for (let at: number | undefined = node; at !== undefined && at >= 0; at = parentIndex[at])
        if (positioned.has(at)) return true
      return false
    }
    const seen = (node: number): boolean => {
      if (unseen.has(node)) return false
      for (let at: number | undefined = node; at !== undefined && at >= 0; at = parentIndex[at])
        if (transparent.has(at)) return false
      return true
    }
    const add = (node: number): void => {
      const id = backendNodeId[node]
      if (id === undefined || actionable.has(id) || hidden.has(id) || !seen(node)) return
      ;(inLayer(node) ? floating : rest).push(id)
    }
    for (const [node, value] of cursor) {
      if (value !== 'pointer' || nodeType[node] !== ELEMENT_NODE) continue
      if (inherited(node) === 'pointer') continue
      const rows = (children.get(node) ?? []).filter(
        (child) => ROWS.has(snapshot.strings[nodeName[child]] ?? '') && cursor.has(child)
      )
      if (rows.some((row) => holdsActionable.has(row))) rows.length = 0
      if (rows.length > 1) rows.forEach(add)
      else add(node)
    }
  }
  return new Set([...floating, ...rest].slice(0, MAX_CLICKABLE))
}

export async function findClickable(
  cdp: Cdp,
  actionable: Set<number>,
  hidden: Set<number>
): Promise<Set<number>> {
  try {
    return pointerOwners(
      await cdp.send<DomSnapshot>('DOMSnapshot.captureSnapshot', { computedStyles: STYLES }),
      actionable,
      hidden
    )
  } catch {
    // A page that cannot be snapshotted just has no extra targets.
    return new Set()
  }
}
