// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

export interface LayoutTab {
  id: string
}

export type LayoutLeaf<T extends LayoutTab> = {
  type: 'leaf'
  id: string
  tabs: T[]
  activeTabId: string | null
}

export type LayoutNode<T extends LayoutTab> =
  | LayoutLeaf<T>
  | {
      type: 'split'
      id: string
      direction: 'row' | 'column'
      children: LayoutNode<T>[]
      sizes: number[]
    }

export type DropZone = 'center' | 'left' | 'right' | 'up' | 'down'
export type EdgeZone = Exclude<DropZone, 'center'>

export function createLeaf<T extends LayoutTab>(tabs: T[] = []): LayoutLeaf<T> {
  return { type: 'leaf', id: crypto.randomUUID(), tabs, activeTabId: tabs[0]?.id ?? null }
}

function replaceNode<T extends LayoutTab>(
  root: LayoutNode<T>,
  targetId: string,
  replace: (node: LayoutNode<T>) => LayoutNode<T> | null
): LayoutNode<T> | null {
  if (root.id === targetId) return replace(root)
  if (root.type !== 'split') return root

  const children = root.children
    .map((child) => replaceNode(child, targetId, replace))
    .filter((child): child is LayoutNode<T> => child !== null)

  if (children.length === 0) return null
  if (children.length === 1) return children[0]

  const sizes =
    children.length === root.children.length ? root.sizes : children.map(() => 1 / children.length)
  return { ...root, children, sizes }
}

export function allLeaves<T extends LayoutTab>(root: LayoutNode<T> | null): LayoutLeaf<T>[] {
  if (!root) return []
  if (root.type === 'leaf') return [root]
  return root.children.flatMap((child) => allLeaves(child))
}

export function findLeaf<T extends LayoutTab>(
  root: LayoutNode<T> | null,
  paneId: string | null
): LayoutLeaf<T> | null {
  return allLeaves(root).find((leaf) => leaf.id === paneId) ?? null
}

export function findLeafByTab<T extends LayoutTab>(
  root: LayoutNode<T> | null,
  tabId: string
): LayoutLeaf<T> | null {
  return allLeaves(root).find((leaf) => leaf.tabs.some((tab) => tab.id === tabId)) ?? null
}

export function firstLeafId<T extends LayoutTab>(root: LayoutNode<T> | null): string | null {
  return allLeaves(root)[0]?.id ?? null
}

export function addTab<T extends LayoutTab>(
  root: LayoutNode<T>,
  paneId: string,
  tab: T
): LayoutNode<T> {
  return (
    replaceNode(root, paneId, (node) =>
      node.type === 'leaf' ? { ...node, tabs: [...node.tabs, tab], activeTabId: tab.id } : node
    ) ?? root
  )
}

export function closeTab<T extends LayoutTab>(
  root: LayoutNode<T>,
  paneId: string,
  tabId: string
): LayoutNode<T> | null {
  return replaceNode(root, paneId, (node) => {
    if (node.type !== 'leaf') return node
    const index = node.tabs.findIndex((tab) => tab.id === tabId)
    if (index === -1) return node
    const tabs = node.tabs.filter((tab) => tab.id !== tabId)
    if (tabs.length === 0) return null
    const activeTabId =
      node.activeTabId === tabId ? tabs[Math.min(index, tabs.length - 1)].id : node.activeTabId
    return { ...node, tabs, activeTabId }
  })
}

export function setActiveTab<T extends LayoutTab>(
  root: LayoutNode<T>,
  paneId: string,
  tabId: string
): LayoutNode<T> {
  return (
    replaceNode(root, paneId, (node) =>
      node.type === 'leaf' ? { ...node, activeTabId: tabId } : node
    ) ?? root
  )
}

export function setSplitSizes<T extends LayoutTab>(
  root: LayoutNode<T>,
  splitId: string,
  sizes: number[]
): LayoutNode<T> {
  return (
    replaceNode(root, splitId, (node) => (node.type === 'split' ? { ...node, sizes } : node)) ??
    root
  )
}

export function splitWithTab<T extends LayoutTab>(
  root: LayoutNode<T>,
  paneId: string,
  zone: EdgeZone,
  tab: T
): LayoutNode<T> {
  const newLeaf = createLeaf([tab])
  const before = zone === 'left' || zone === 'up'
  const direction = zone === 'left' || zone === 'right' ? 'row' : 'column'
  return (
    replaceNode(root, paneId, (node) => ({
      type: 'split',
      id: crypto.randomUUID(),
      direction,
      children: before ? [newLeaf, node] : [node, newLeaf],
      sizes: [0.5, 0.5]
    })) ?? root
  )
}

// Moves or duplicates a tab into a pane (center) or into a new pane split off its edge.
// `placed` is the tab as it exists at its destination.
export function placeTab<T extends LayoutTab>(
  root: LayoutNode<T>,
  fromPaneId: string,
  tabId: string,
  toPaneId: string,
  zone: DropZone,
  duplicate: boolean
): { root: LayoutNode<T>; placed: T } | null {
  const source = findLeaf(root, fromPaneId)
  const tab = source?.tabs.find((t) => t.id === tabId)
  if (!source || !tab) return null

  if (!duplicate && fromPaneId === toPaneId && (zone === 'center' || source.tabs.length === 1)) {
    return null
  }

  const placed: T = duplicate ? { ...tab, id: crypto.randomUUID() } : tab
  const next =
    zone === 'center' ? addTab(root, toPaneId, placed) : splitWithTab(root, toPaneId, zone, placed)
  if (duplicate) return { root: next, placed }
  const trimmed = closeTab(next, fromPaneId, tabId)
  return trimmed ? { root: trimmed, placed } : null
}
