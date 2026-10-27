// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

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

// A sidebar link that sets the pointer (it already has a ref from its role), then a dropdown layer appended to
// body holding a list whose rows inherit the pointer from it.
const dropdown: DomSnapshot = {
  strings: ['', 'default', 'pointer', 'static', 'absolute', 'A', 'DIV', 'UL', 'LI'],
  documents: [
    {
      nodes: {
        // document, link, layer, list, row, row, row text
        parentIndex: [-1, 0, 0, 2, 3, 3, 4],
        backendNodeId: [1, 2, 3, 4, 5, 6, 7],
        nodeType: [9, 1, 1, 1, 1, 1, 3],
        nodeName: [0, 5, 6, 7, 8, 8, 0]
      },
      layout: {
        nodeIndex: [1, 2, 3, 4, 5, 6],
        styles: [
          [2, 3],
          [1, 4],
          [2, 3],
          [2, 3],
          [2, 3],
          [2, 3]
        ]
      }
    }
  ]
}

test('elements with an actionable role do not count, and rows of a pointer list are the targets', () => {
  assert.deepEqual([...pointerOwners(dropdown, new Set([2]))], [5, 6])
})

test('elements in a floating layer come before the rest when the cap cuts', () => {
  const many: DomSnapshot = {
    strings: ['', 'pointer', 'static', 'fixed', 'DIV'],
    documents: [
      {
        nodes: {
          parentIndex: [-1, ...Array.from({ length: 70 }, () => 0), 0, 71],
          backendNodeId: Array.from({ length: 73 }, (_, i) => i + 1),
          nodeType: [9, ...Array.from({ length: 72 }, () => 1)],
          nodeName: Array.from({ length: 73 }, () => 4)
        },
        layout: {
          nodeIndex: Array.from({ length: 72 }, (_, i) => i + 1),
          styles: [...Array.from({ length: 70 }, () => [1, 2]), [0, 3], [1, 2]]
        }
      }
    ]
  }
  const owners = [...pointerOwners(many)]
  assert.equal(owners.length, 60)
  assert.equal(owners[0], 73)
})

// A visible tab, a zero size element, a hidden one, and a panel at opacity 0 holding a pointer item, as a closed
// offcanvas or a toast waiting to fade in leaves in the page.
const unseen: DomSnapshot = {
  strings: ['', 'pointer', 'default', '1', '0', 'visible', 'hidden'],
  documents: [
    {
      nodes: {
        // document, tab, zero size, hidden, faded panel, item inside it
        parentIndex: [-1, 0, 0, 0, 0, 4],
        backendNodeId: [1, 2, 3, 4, 5, 6],
        nodeType: [9, 1, 1, 1, 1, 1]
      },
      layout: {
        nodeIndex: [1, 2, 3, 4, 5],
        styles: [
          [1, 0, 3, 5],
          [1, 0, 3, 5],
          [1, 0, 3, 6],
          [2, 0, 4, 5],
          [1, 0, 3, 5]
        ],
        bounds: [
          [0, 0, 80, 20],
          [0, 0, 0, 0],
          [0, 0, 80, 20],
          [0, 0, 300, 600],
          [0, 0, 80, 20]
        ]
      }
    }
  ]
}

test('elements a person cannot see are not clickable', () => {
  assert.deepEqual([...pointerOwners(unseen)], [2])
})

test('elements the page hides with aria-hidden or inert are not clickable', () => {
  const owners = pointerOwners(snapshot, new Set(), new Set([7]))
  assert.deepEqual([...owners], [10])
})
