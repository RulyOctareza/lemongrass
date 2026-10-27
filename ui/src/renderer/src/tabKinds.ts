// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import type { Component } from 'vue'
import DeviceView from './components/DeviceView.vue'
import DiffViewer from './components/DiffViewer.vue'
import DocView from './components/DocView.vue'
import HistoryView from './components/HistoryView.vue'
import ShellView from './components/ShellView.vue'
import VaultPanel from './components/VaultPanel.vue'
import WebPane from './components/WebPane.vue'
import WorkgroupPane from './components/WorkgroupPane.vue'
import { shellTitles } from './shellRegistry'
import { workgroupStateOf } from './workgroups'
import type { ProjectRef } from './workspace'
import type { WorkspaceTab } from '../../preload/types'

export interface IconShape {
  tag: 'path' | 'rect' | 'circle'
  attrs: Record<string, string | number>
}

export interface TabLabel {
  prefix?: string
  text: string
}

export interface TabContext {
  project: ProjectRef
  active: boolean
}

type KindName = WorkspaceTab['kind']
type TabOf<K extends KindName> = Extract<WorkspaceTab, { kind: K }>

interface TabKind<T extends WorkspaceTab> {
  icon: IconShape[]
  label: (tab: T, ctx: TabContext) => TabLabel
  title: (tab: T, ctx: TabContext) => string
  body: Component
  // A kept alive body stays mounted while its tab is in the background.
  keepAlive: boolean
  bodyProps: (tab: T, ctx: TabContext) => Record<string, unknown>
}

function pathLabel(path: string): TabLabel {
  const segments = path.replace(/\.md$/, '').split('/')
  return { prefix: segments[segments.length - 2], text: segments[segments.length - 1] }
}

function shellLabel(tab: TabOf<'shell'>): string {
  return shellTitles[tab.id] ?? tab.label
}

const kinds: { [K in KindName]: TabKind<TabOf<K>> } = {
  shell: {
    icon: [{ tag: 'path', attrs: { d: 'M2.5 4.5l3 2.5-3 2.5M7 10h4.5' } }],
    label: (tab) => ({ text: shellLabel(tab) }),
    title: shellLabel,
    body: ShellView,
    keepAlive: true,
    bodyProps: (tab) => ({
      spec: {
        id: tab.id,
        command: tab.command,
        cwd: tab.cwd,
        account: tab.account,
        thinker: tab.thinker
      }
    })
  },
  doc: {
    icon: [],
    label: (tab) => pathLabel(tab.path),
    title: (tab) => tab.path,
    body: DocView,
    keepAlive: false,
    bodyProps: (tab, ctx) => ({ project: ctx.project, tab })
  },
  diff: {
    icon: [
      { tag: 'rect', attrs: { x: 1.5, y: 2, width: 11, height: 10, rx: 1.5 } },
      { tag: 'path', attrs: { d: 'M7 2v10M3.5 5.5h2M9 8.5h2' } }
    ],
    label: (tab) => ({ prefix: 'diff', text: tab.path.slice(tab.path.lastIndexOf('/') + 1) }),
    title: (tab) => `Diff: ${tab.path}`,
    body: DiffViewer,
    keepAlive: false,
    bodyProps: (tab, ctx) => ({ project: ctx.project, path: tab.path })
  },
  history: {
    icon: [
      { tag: 'circle', attrs: { cx: 7, cy: 7, r: 5.2 } },
      { tag: 'path', attrs: { d: 'M7 4v3.2l2.2 1.3' } }
    ],
    label: () => ({ text: 'History' }),
    title: () => 'Git history',
    body: HistoryView,
    keepAlive: true,
    bodyProps: (_tab, ctx) => ({ project: ctx.project })
  },
  // The persisted kind stays 'connector' so saved layouts keep loading.
  connector: {
    icon: [
      { tag: 'rect', attrs: { x: 2.5, y: 6.5, width: 9, height: 6, rx: 1.2 } },
      { tag: 'path', attrs: { d: 'M4.5 6.5V4.5a2.5 2.5 0 0 1 5 0v2' } },
      { tag: 'circle', attrs: { cx: 7, cy: 9.5, r: 0.8 } }
    ],
    label: () => ({ text: 'Vault' }),
    title: () => 'Vault',
    body: VaultPanel,
    keepAlive: true,
    bodyProps: (_tab, ctx) => ({ active: ctx.active })
  },
  device: {
    icon: [
      { tag: 'rect', attrs: { x: 3.5, y: 1.5, width: 7, height: 11, rx: 1.4 } },
      { tag: 'path', attrs: { d: 'M6 10.5h2' } }
    ],
    label: () => ({ text: 'Device' }),
    title: () => 'Device',
    body: DeviceView,
    // Kept alive so switching tabs does not restart the mirror on the phone.
    keepAlive: true,
    bodyProps: () => ({})
  },
  workgroup: {
    icon: [
      { tag: 'circle', attrs: { cx: 7, cy: 3.5, r: 1.7 } },
      { tag: 'circle', attrs: { cx: 3, cy: 10.5, r: 1.7 } },
      { tag: 'circle', attrs: { cx: 11, cy: 10.5, r: 1.7 } },
      { tag: 'path', attrs: { d: 'M6 5l-2 4M8 5l2 4M4.7 10.5h4.6' } }
    ],
    label: (tab, ctx) => ({ prefix: 'workgroup', text: workgroupName(tab, ctx) }),
    title: (tab, ctx) => workgroupName(tab, ctx),
    body: WorkgroupPane,
    keepAlive: true,
    bodyProps: (tab, ctx) => ({ project: ctx.project, groupId: tab.groupId, active: ctx.active })
  },
  web: {
    icon: [
      { tag: 'circle', attrs: { cx: 7, cy: 7, r: 5.2 } },
      {
        tag: 'path',
        attrs: {
          d: 'M1.8 7h10.4M7 1.8c-1.6 1.6-2.4 3.3-2.4 5.2s.8 3.6 2.4 5.2c1.6-1.6 2.4-3.3 2.4-5.2S8.6 3.4 7 1.8Z'
        }
      }
    ],
    label: (tab) => ({ prefix: 'web', text: shellTitles[tab.shellTabId] ?? 'browser' }),
    title: (tab) => `Browser of ${shellTitles[tab.shellTabId] ?? 'a shell tab'}`,
    body: WebPane,
    // The native view is attached only while the tab is active, so the body can unmount.
    keepAlive: false,
    bodyProps: (tab) => ({ shellTabId: tab.shellTabId })
  }
}

function workgroupName(tab: TabOf<'workgroup'>, ctx: TabContext): string {
  return workgroupStateOf(ctx.project.id).names[tab.groupId] ?? 'Workgroup'
}

export function tabKindOf(tab: WorkspaceTab): TabKind<WorkspaceTab> {
  return kinds[tab.kind] as unknown as TabKind<WorkspaceTab>
}
