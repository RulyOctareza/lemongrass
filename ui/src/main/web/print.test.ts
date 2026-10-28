// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import assert from 'node:assert/strict'
import { test } from 'node:test'
import { Baseline, printChanges, printFull } from './print.ts'
import { RefBook, buildTree, type AXNode } from './snapshot.ts'

const v = (value: unknown): { value: unknown } => ({ value })
const header = { url: 'http://x/', title: 'T' }

// A navigation of `links` links, a form with a textbox holding `value`, and optionally a modal dialog.
function page(links: number, value: string, dialog = false): AXNode[] {
  const nodes: AXNode[] = [
    { nodeId: 'root', role: v('RootWebArea'), backendDOMNodeId: 1, childIds: ['nav', 'form'] },
    {
      nodeId: 'nav',
      role: v('navigation'),
      name: v('Sidebar'),
      backendDOMNodeId: 2,
      parentId: 'root',
      childIds: ['list']
    },
    { nodeId: 'list', role: v('list'), backendDOMNodeId: 3, parentId: 'nav', childIds: [] },
    {
      nodeId: 'form',
      role: v('form'),
      name: v('Edit'),
      backendDOMNodeId: 4,
      parentId: 'root',
      childIds: ['box', 'save']
    },
    {
      nodeId: 'box',
      role: v('textbox'),
      name: v('Name'),
      value: v(value),
      backendDOMNodeId: 5,
      parentId: 'form'
    },
    { nodeId: 'save', role: v('button'), name: v('Save'), backendDOMNodeId: 6, parentId: 'form' }
  ]
  for (let i = 0; i < links; i++) {
    nodes[2].childIds?.push(`li${i}`)
    nodes.push({
      nodeId: `li${i}`,
      role: v('listitem'),
      backendDOMNodeId: 100 + i,
      parentId: 'list',
      childIds: [`a${i}`]
    })
    nodes.push({
      nodeId: `a${i}`,
      role: v('link'),
      name: v(`Menu ${i + 1}`),
      backendDOMNodeId: 200 + i,
      parentId: `li${i}`
    })
  }
  if (dialog) {
    nodes[0].childIds?.push('dlg')
    nodes.push({
      nodeId: 'dlg',
      role: v('dialog'),
      name: v('Create'),
      backendDOMNodeId: 7,
      parentId: 'root',
      childIds: ['close'],
      properties: [{ name: 'modal', value: v(true) }]
    })
    nodes.push({
      nodeId: 'close',
      role: v('button'),
      name: v('Close'),
      backendDOMNodeId: 8,
      parentId: 'dlg'
    })
  }
  return nodes
}

function session(): {
  full: (n: AXNode[]) => string
  changes: (n: AXNode[]) => string
  book: RefBook
} {
  const book = new RefBook()
  const baseline = new Baseline()
  return {
    book,
    full: (n) => printFull(buildTree(n, { masked: new Set() }, book), header, book, baseline),
    changes: (n) => printChanges(buildTree(n, { masked: new Set() }, book), header, book, baseline)
  }
}

test('an action result prints only the changed line, its ancestors and one fold line per unchanged run', () => {
  const s = session()
  s.full(page(5, 'a'))
  const out = s.changes(page(5, 'ab'))
  assert.match(out, /^navigation "Sidebar" \[ref=e\d+\]: unchanged, 5 refs$/m)
  assert.match(out, /^form "Edit"$/m)
  assert.match(out, /^ {2}textbox "Name" \[ref=e6\] value="ab"$/m)
  assert.match(out, /^ {2}\(1 unchanged, 1 ref\)$/m)
  assert.ok(!out.includes('Menu 1'))
  assert.ok(!out.includes('Save'))
})

test('a page with nothing changed is one line, and refs inside folds stay valid', () => {
  const s = session()
  s.full(page(5, 'a'))
  const out = s.changes(page(5, 'a'))
  assert.match(out, /^page: unchanged, 7 refs$/m)
  assert.equal(s.book.lookup('e1').name, 'Menu 1')
})

test('a modal dialog prints alone, and the page behind it prints its changes once it closes', () => {
  const s = session()
  s.full(page(3, 'a'))
  const open = s.changes(page(3, 'b', true))
  assert.match(open, /a modal dialog is open/)
  assert.match(open, /^dialog "Create"$/m)
  assert.match(open, /button "Close"/)
  assert.ok(!open.includes('textbox'))
  const closed = s.changes(page(3, 'b'))
  assert.match(closed, /textbox "Name" \[ref=e4\] value="b"/)
  assert.match(closed, /^removed: dialog "Create" \(1 ref\)$/m)
})

test('a long list prints 20 items and a count with a ref to open it', () => {
  const s = session()
  const out = s.full(page(50, 'a'))
  assert.ok(out.includes('Menu 20'))
  assert.ok(!out.includes('Menu 21'))
  assert.match(out, /^ {4}\.\.\. 30 more items, type to filter or snapshot --within e\d+$/m)
  assert.equal(s.book.lookup('e45').name, 'Menu 45')
})
