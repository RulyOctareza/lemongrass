// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import * as tree from './layoutTree.ts'
import type { WorkspaceTab } from '../../preload/types.ts'

const shell: WorkspaceTab = {
  id: 'shell-1',
  kind: 'shell',
  label: 'Claude Code 1',
  command: 'claude'
}
const web: WorkspaceTab = { id: 'web-1', kind: 'web', shellTabId: 'shell-1' }

function layoutWithWeb(): tree.LayoutNode<WorkspaceTab> {
  const leaf = tree.createLeaf<WorkspaceTab>([shell])
  return tree.splitWithTab(leaf, leaf.id, 'right', web)
}

test('a web tab opens in a new pane beside its shell', () => {
  const root = layoutWithWeb()
  assert.equal(tree.allLeaves(root).length, 2)
  assert.equal(tree.findLeafByTab(root, 'web-1')?.activeTabId, 'web-1')
})

test('a layout with a web tab survives a save and restore', () => {
  const root = layoutWithWeb()
  const restored = JSON.parse(JSON.stringify({ root, focusedPaneId: null })).root
  assert.deepEqual(restored, root)
  const tab = tree.findLeafByTab(restored, 'web-1')?.tabs[0]
  assert.deepEqual(tab, web)
})

test('closing the shell keeps the web tab and closing the web tab keeps the shell', () => {
  const root = layoutWithWeb()
  const shellPane = tree.findLeafByTab(root, 'shell-1')!.id
  const withoutShell = tree.closeTab(root, shellPane, 'shell-1')
  assert.deepEqual(
    tree.allLeaves(withoutShell).flatMap((leaf) => leaf.tabs.map((tab) => tab.id)),
    ['web-1']
  )
  const webPane = tree.findLeafByTab(root, 'web-1')!.id
  const withoutWeb = tree.closeTab(root, webPane, 'web-1')
  assert.deepEqual(
    tree.allLeaves(withoutWeb).flatMap((leaf) => leaf.tabs.map((tab) => tab.id)),
    ['shell-1']
  )
})
