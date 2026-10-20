import assert from 'node:assert/strict'
import { test } from 'node:test'
import { pointerOwners, type DomSnapshot } from './clickable.ts'

// Captured from a page with a default cursor strip holding two pointer tabs and plain text, where the first
// tab has a span inside it that only inherits the pointer.
const snapshot: DomSnapshot = {
  strings: ['', 'default', 'pointer'],
  documents: [
    {
      nodes: {
        // document, strip, tab, tab's span, tab's text, second tab, plain
        parentIndex: [-1, 0, 1, 2, 3, 1, 1],
        backendNodeId: [1, 6, 7, 8, 9, 10, 11],
        nodeType: [9, 1, 1, 1, 3, 1, 1]
      },
      layout: {
        nodeIndex: [1, 2, 3, 4, 5, 6],
        styles: [[1], [2], [2], [2], [2], [1]]
      }
    }
  ]
}

test('only elements that set the pointer cursor themselves are clickable', () => {
  assert.deepEqual(
    [...pointerOwners(snapshot)].sort((a, b) => a - b),
    [7, 10]
  )
})

test('a page with no layout has no clickable elements', () => {
  assert.equal(
    pointerOwners({
      strings: [],
      documents: [{ nodes: {}, layout: { nodeIndex: [], styles: [] } }]
    }).size,
    0
  )
})
