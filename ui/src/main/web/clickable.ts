import type { Cdp } from './cdp'

interface DocumentSnapshot {
  nodes: { parentIndex?: number[]; backendNodeId?: number[]; nodeType?: number[] }
  layout: { nodeIndex: number[]; styles: number[][] }
}

export interface DomSnapshot {
  documents: DocumentSnapshot[]
  strings: string[]
}

const ELEMENT_NODE = 1
// A page full of pointer cursors, such as a long table of clickable rows, would bury everything else.
const MAX_CLICKABLE = 60

// Elements that set the pointer cursor themselves: the cursor is the page's own sign that a click does
// something here. An element whose cursor is only inherited from a pointer ancestor is left out, so a clickable
// card gives one target and not one per word inside it.
export function pointerOwners(snapshot: DomSnapshot): Set<number> {
  const owners = new Set<number>()
  for (const doc of snapshot.documents) {
    const { parentIndex = [], backendNodeId = [], nodeType = [] } = doc.nodes
    const cursor = new Map<number, string>()
    doc.layout.nodeIndex.forEach((node, i) => {
      cursor.set(node, snapshot.strings[doc.layout.styles[i]?.[0] ?? -1] ?? '')
    })
    const inherited = (node: number): string => {
      for (let at = parentIndex[node]; at !== undefined && at >= 0; at = parentIndex[at]) {
        const found = cursor.get(at)
        if (found !== undefined) return found
      }
      return ''
    }
    for (const [node, value] of cursor) {
      if (value !== 'pointer' || nodeType[node] !== ELEMENT_NODE) continue
      const id = backendNodeId[node]
      if (id !== undefined && inherited(node) !== 'pointer') owners.add(id)
    }
  }
  return new Set([...owners].slice(0, MAX_CLICKABLE))
}

export async function findClickable(cdp: Cdp): Promise<Set<number>> {
  try {
    return pointerOwners(
      await cdp.send<DomSnapshot>('DOMSnapshot.captureSnapshot', {
        computedStyles: ['cursor']
      })
    )
  } catch {
    // A page that cannot be snapshotted just has no extra targets.
    return new Set()
  }
}
