// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { nextTick, reactive, watch } from 'vue'
import { marked } from 'marked'
import * as tree from './layoutTree'
import type { DropZone, EdgeZone } from './layoutTree'
import {
  disposeShell,
  focusShell,
  onShellAccountChange,
  onShellExit,
  setShellArgs,
  setShellResume,
  startShellDetached
} from './shellRegistry'
import { accountIsDeleted, preferredAccount, setPreferredAccount } from './accounts'
import { defaultGroupColor, groupColorVar, isGroupColor, type GroupColor } from './groupColors'
import { isWorkgroupMember, tabGroup, workgroupStateOf } from './workgroups'
import {
  preferredShellAgentId,
  resumeArgs,
  setPreferredShellAgentId,
  shellAgentById,
  type ShellAgent
} from './shellAgents'
import type {
  WorkgroupSpawnMember,
  WorkgroupSpawnRequest,
  WorkgroupSpawnResult,
  WorkspaceLayoutState,
  WorkspaceTab
} from '../../preload/types'

export interface ProjectRef {
  id: string
  path: string
}

// Only markdown is rendered or edited; other scratchpad files open in a read-only viewer chosen by kind.
export type DocKind = 'markdown' | 'text' | 'image' | 'binary' | 'tooLarge'

export interface DocEntry {
  kind: DocKind
  editable: boolean
  // Raw markdown for editable docs, raw text for a text file.
  content: string
  html: string
  // The image's mtime, appended to its URL so a rewritten image is not served from cache.
  imageVersion: number
  status: 'loading' | 'ready' | 'missing'
  // Last content known to match disk, for editable docs -- lets flushDoc
  // tell an external write (another pane, a model) apart from its own echo.
  baseline: string
}

interface Workspace {
  layout: WorkspaceLayoutState
  docs: Record<string, DocEntry>
}

type DocTab = Extract<WorkspaceTab, { kind: 'doc' }>
type DiffTab = Extract<WorkspaceTab, { kind: 'diff' }>
type HistoryTab = Extract<WorkspaceTab, { kind: 'history' }>
type VaultTab = Extract<WorkspaceTab, { kind: 'connector' }>
type ShellTab = Extract<WorkspaceTab, { kind: 'shell' }>
type WorkgroupTab = Extract<WorkspaceTab, { kind: 'workgroup' }>
type WebTab = Extract<WorkspaceTab, { kind: 'web' }>
type DeviceTab = Extract<WorkspaceTab, { kind: 'device' }>

const AUTOSAVE_MS = 600
const LAYOUT_SAVE_MS = 400

export type ShellPlacement =
  | { kind: 'tab'; project: ProjectRef; paneId?: string }
  | { kind: 'split'; project: ProjectRef; paneId: string; zone: EdgeZone }
  | { kind: 'openDoc'; project: ProjectRef; paneId: string; tabId: string }

export const shellPicker = reactive<{
  open: boolean
  placement: ShellPlacement | null
  selectedId: string
  account: string | undefined
}>({
  open: false,
  placement: null,
  selectedId: preferredShellAgentId(),
  account: undefined
})

const workspaces = reactive<Record<string, Workspace>>({})
const projectRefs = new Map<string, ProjectRef>()
const initPromises = new Map<string, Promise<void>>()
const docSaveTimers = new Map<string, ReturnType<typeof setTimeout>>()
const layoutSaveTimers = new Map<string, ReturnType<typeof setTimeout>>()

export function workspaceOf(projectId: string): Workspace | undefined {
  return workspaces[projectId]
}

export function isMarkdownPath(path: string): boolean {
  return path.endsWith('.md')
}

export function isEditablePath(path: string): boolean {
  return (
    isMarkdownPath(path) &&
    path.startsWith('scratchpad/') &&
    !path.startsWith('scratchpad/archive/')
  )
}

export function projectRelativePath(path: string): string {
  return `biblio/${path}`
}

function newDocTab(path: string): DocTab {
  return { id: crypto.randomUUID(), kind: 'doc', path }
}

function newDiffTab(path: string): DiffTab {
  return { id: crypto.randomUUID(), kind: 'diff', path }
}

function newHistoryTab(): HistoryTab {
  return { id: crypto.randomUUID(), kind: 'history' }
}

// The persisted tab kind stays 'connector' so saved layouts keep loading.
function newVaultTab(): VaultTab {
  return { id: crypto.randomUUID(), kind: 'connector' }
}

function newDeviceTab(): DeviceTab {
  return { id: crypto.randomUUID(), kind: 'device' }
}

function newWorkgroupTab(groupId: number): WorkgroupTab {
  return { id: crypto.randomUUID(), kind: 'workgroup', groupId }
}

function newWebTab(shellTabId: string): WebTab {
  return { id: crypto.randomUUID(), kind: 'web', shellTabId }
}

function tabsOf(projectId: string): WorkspaceTab[] {
  return tree.allLeaves(workspaces[projectId].layout.root).flatMap((leaf) => leaf.tabs)
}

function newShellTab(project: ProjectRef, agent: ShellAgent, account?: string): ShellTab {
  const count = tabsOf(project.id).filter(
    (tab) => tab.kind === 'shell' && tab.command === agent.command
  ).length
  return {
    id: crypto.randomUUID(),
    kind: 'shell',
    label: `${agent.label} ${count + 1}`,
    command: agent.command,
    cwd: project.path,
    ...(account ? { account } : {})
  }
}

function focusShellAfterMount(tabId: string): void {
  void nextTick(() => focusShell(tabId))
}

export function openShellPicker(placement: ShellPlacement): void {
  shellPicker.placement = placement
  shellPicker.selectedId = preferredShellAgentId()
  shellPicker.account = preferredAccount()
  shellPicker.open = true
}

export function cancelShellPicker(): void {
  shellPicker.open = false
  shellPicker.placement = null
}

export function confirmShellPicker(agentId?: string): void {
  const placement = shellPicker.placement
  if (!placement) return
  const agent = shellAgentById(agentId ?? shellPicker.selectedId)
  setPreferredShellAgentId(agent.id)
  const account = agent.id === 'claude' ? shellPicker.account : undefined
  if (agent.id === 'claude') setPreferredAccount(account)
  shellPicker.open = false
  shellPicker.placement = null
  shellPicker.selectedId = agent.id

  if (placement.kind === 'tab') {
    addShellWithAgent(placement.project, agent, placement.paneId, account)
    return
  }
  if (placement.kind === 'split') {
    addShellInNewPaneWithAgent(placement.project, placement.paneId, placement.zone, agent, account)
    return
  }
  openInNewShellWithAgent(placement.project, placement.paneId, placement.tabId, agent, account)
}

function openPaths(projectId: string): string[] {
  const paths = tabsOf(projectId).flatMap((tab) => (tab.kind === 'doc' ? [tab.path] : []))
  return [...new Set(paths)]
}

export function isTabOpen(projectId: string, tabId: string): boolean {
  return workspaces[projectId] ? tabsOf(projectId).some((tab) => tab.id === tabId) : false
}

export function focusTab(project: ProjectRef, tabId: string): void {
  const root = workspaces[project.id]?.layout.root ?? null
  const leaf = tree.findLeafByTab(root, tabId)
  if (root && leaf) commit(project, tree.setActiveTab(root, leaf.id, tabId), leaf.id)
}

export function groupColorOf(projectId: string, groupId: number): GroupColor {
  const chosen = workspaces[projectId]?.layout.groupColors?.[String(groupId)]
  return isGroupColor(chosen) ? chosen : defaultGroupColor(groupId)
}

export function setGroupColor(projectId: string, groupId: number, color: GroupColor): void {
  const layout = workspaces[projectId]?.layout
  if (!layout) return
  layout.groupColors = { ...layout.groupColors, [String(groupId)]: color }
}

export interface TabBand {
  color: string
  leader: boolean
  groupName: string
}

export function tabBand(projectId: string, tab: WorkspaceTab): TabBand | null {
  const group = tabGroup(projectId, tab)
  if (!group) return null
  return {
    color: groupColorVar(groupColorOf(projectId, group.id)),
    leader: group.leader,
    groupName: workgroupStateOf(projectId).names[group.id] ?? 'Workgroup'
  }
}

export function focusedWorkgroupId(projectId: string): number | null {
  const layout = workspaces[projectId]?.layout
  if (!layout) return null
  const leaf = tree.findLeaf(layout.root, layout.focusedPaneId)
  const tab = leaf?.tabs.find((t) => t.id === leaf.activeTabId)
  return tab?.kind === 'workgroup' ? tab.groupId : null
}

export function liveShellCount(projectId: string): number {
  return workspaces[projectId] ? tabsOf(projectId).filter((t) => t.kind === 'shell').length : 0
}

export function docTabCount(projectId: string): number {
  return workspaces[projectId] ? tabsOf(projectId).filter((t) => t.kind === 'doc').length : 0
}

async function loadDoc(project: ProjectRef, path: string, force = false): Promise<void> {
  const workspace = workspaces[project.id]
  const existing = workspace.docs[path]
  if (existing?.status === 'ready' && !force) return
  if (existing?.status === 'ready' && existing.editable) {
    const raw = await window.api.biblio.read(project.path, path)
    if (raw === null) markMissing(project, path)
    else if (raw !== existing.baseline && !docSaveTimers.has(`${project.id}:${path}`)) {
      existing.content = raw
      existing.baseline = raw
    }
    return
  }

  const editable = isEditablePath(path)
  if (!existing) {
    workspace.docs[path] = {
      kind: 'markdown',
      editable,
      content: '',
      html: '',
      imageVersion: 0,
      status: 'loading',
      baseline: ''
    }
  }
  if (!isMarkdownPath(path)) {
    await loadFile(project, path)
    return
  }
  const raw = await window.api.biblio.read(project.path, path)
  const entry = workspaces[project.id]?.docs[path]
  if (!entry) return
  if (raw === null) {
    entry.status = 'missing'
    return
  }
  if (editable) {
    entry.content = raw
    entry.baseline = raw
  } else entry.html = await marked.parse(raw)
  entry.status = 'ready'
}

async function loadFile(project: ProjectRef, path: string): Promise<void> {
  const result = await window.api.biblio.readFile(project.path, path)
  const entry = workspaces[project.id]?.docs[path]
  if (!entry) return
  if (result === null) {
    entry.status = 'missing'
    return
  }
  entry.kind = result.kind
  entry.content = result.kind === 'text' ? result.text : ''
  entry.imageVersion = result.kind === 'image' ? result.modifiedMs : 0
  entry.status = 'ready'
}

function markMissing(project: ProjectRef, path: string): void {
  const key = `${project.id}:${path}`
  clearTimeout(docSaveTimers.get(key))
  docSaveTimers.delete(key)
  const doc = workspaces[project.id]?.docs[path]
  if (doc) doc.status = 'missing'
}

async function flushDoc(project: ProjectRef, path: string): Promise<void> {
  const key = `${project.id}:${path}`
  const timer = docSaveTimers.get(key)
  if (!timer) return
  clearTimeout(timer)
  docSaveTimers.delete(key)
  const doc = workspaces[project.id]?.docs[path]
  if (!doc || doc.status !== 'ready') return

  // The file may have changed on disk since we last synced it -- another
  // pane or a model writing to the same scratchpad file. Saving our buffer
  // in that case would silently discard that write, so disk wins instead.
  const onDisk = await window.api.biblio.read(project.path, path)
  const current = workspaces[project.id]?.docs[path]
  if (!current) return
  if (onDisk === null) {
    markMissing(project, path)
    return
  }
  if (onDisk !== current.baseline) {
    current.content = onDisk
    current.baseline = onDisk
    return
  }

  if (current.content === current.baseline) return
  const ok = await window.api.biblio.write(project.path, path, current.content)
  if (ok) current.baseline = current.content
}

export function editDoc(project: ProjectRef, path: string, value: string): void {
  const doc = workspaces[project.id]?.docs[path]
  if (!doc || doc.content === value) return
  doc.content = value
  const key = `${project.id}:${path}`
  clearTimeout(docSaveTimers.get(key))
  docSaveTimers.set(
    key,
    setTimeout(() => flushDoc(project, path), AUTOSAVE_MS)
  )
}

function scheduleLayoutSave(projectId: string): void {
  clearTimeout(layoutSaveTimers.get(projectId))
  layoutSaveTimers.set(
    projectId,
    setTimeout(() => {
      const layout = workspaces[projectId].layout
      window.api.workspaceLayouts.save(
        projectId,
        layout.root || Object.keys(layout.groupColors ?? {}).length
          ? JSON.parse(JSON.stringify(layout))
          : null
      )
    }, LAYOUT_SAVE_MS)
  )
}

function pruneDocs(project: ProjectRef): void {
  const workspace = workspaces[project.id]
  const used = new Set(openPaths(project.id))
  for (const path of Object.keys(workspace.docs)) {
    if (used.has(path)) continue
    flushDoc(project, path)
    delete workspace.docs[path]
  }
}

function commit(
  project: ProjectRef,
  root: tree.LayoutNode<WorkspaceTab> | null,
  focusPaneId?: string | null
): void {
  const layout = workspaces[project.id].layout
  layout.root = root
  if (focusPaneId !== undefined) layout.focusedPaneId = focusPaneId
  const leaves = tree.allLeaves(root)
  if (!leaves.some((leaf) => leaf.id === layout.focusedPaneId)) {
    layout.focusedPaneId = leaves[0]?.id ?? null
  }
  pruneDocs(project)
}

async function refreshDocs(project: ProjectRef): Promise<void> {
  const workspace = workspaces[project.id]
  const paths = new Set([...openPaths(project.id), ...Object.keys(workspace.docs)])
  await Promise.all([...paths].map((path) => loadDoc(project, path, true)))
}

async function restoreShellSessions(
  project: ProjectRef,
  root: tree.LayoutNode<WorkspaceTab> | null
): Promise<void> {
  const shellTabs = tree
    .allLeaves(root)
    .flatMap((leaf) => leaf.tabs)
    .filter((tab): tab is ShellTab => tab.kind === 'shell')
  const sessions = await window.api.tabSessions.list(project.path)
  for (const tab of shellTabs) {
    const sessionId = sessions[tab.id]
    const args = sessionId ? resumeArgs(tab.command, sessionId) : null
    if (args) setShellResume(tab.id, args)
  }
}

async function init(project: ProjectRef): Promise<void> {
  const saved = await window.api.workspaceLayouts.load(project.id)
  await window.api.tabSessions.clear(project.path)
  await restoreShellSessions(project, saved?.root ?? null)
  workspaces[project.id] = {
    layout: {
      root: saved?.root ?? null,
      focusedPaneId: saved?.focusedPaneId ?? null,
      groupColors: saved?.groupColors ?? {}
    },
    docs: {}
  }
  const layout = workspaces[project.id].layout
  const leaves = tree.allLeaves(layout.root)
  if (!leaves.some((leaf) => leaf.id === layout.focusedPaneId)) {
    layout.focusedPaneId = leaves[0]?.id ?? null
  }
  await Promise.all(openPaths(project.id).map((path) => loadDoc(project, path)))
  watch(
    () => workspaces[project.id].layout,
    () => scheduleLayoutSave(project.id),
    { deep: true }
  )
}

// First call loads the saved layout; later calls only refresh documents.
export function ensureLoaded(project: ProjectRef): Promise<void> {
  projectRefs.set(project.id, project)
  const existing = initPromises.get(project.id)
  if (existing) return existing.then(() => refreshDocs(project))
  const loading = init(project)
  initPromises.set(project.id, loading)
  return loading
}

export function refreshReadOnlyDocs(project: ProjectRef): Promise<void> {
  const initialized = initPromises.get(project.id)
  return initialized ? initialized.then(() => refreshDocs(project)) : Promise.resolve()
}

export function activePath(projectId: string): string | null {
  const layout = workspaces[projectId]?.layout
  if (!layout) return null
  const leaf = tree.findLeaf(layout.root, layout.focusedPaneId)
  const tab = leaf?.tabs.find((t) => t.id === leaf.activeTabId)
  return tab?.kind === 'doc' ? tab.path : null
}

export function openFile(project: ProjectRef, path: string): void {
  const layout = workspaces[project.id].layout
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([newDocTab(path)])
    commit(project, leaf, leaf.id)
    void loadDoc(project, path)
    return
  }
  const leaves = tree.allLeaves(root)
  const focused = leaves.find((leaf) => leaf.id === layout.focusedPaneId) ?? leaves[0]
  const hasPath = (leaf: (typeof leaves)[number]): boolean =>
    leaf.tabs.some((tab) => tab.kind === 'doc' && tab.path === path)
  const holder = hasPath(focused) ? focused : leaves.find(hasPath)
  if (holder) {
    const tab = holder.tabs.find((t) => t.kind === 'doc' && t.path === path)!
    commit(project, tree.setActiveTab(root, holder.id, tab.id), holder.id)
    return
  }
  commit(project, tree.addTab(root, focused.id, newDocTab(path)), focused.id)
  void loadDoc(project, path)
}

export function activeDiffPath(projectId: string): string | null {
  const layout = workspaces[projectId]?.layout
  if (!layout) return null
  const leaf = tree.findLeaf(layout.root, layout.focusedPaneId)
  const tab = leaf?.tabs.find((t) => t.id === leaf.activeTabId)
  return tab?.kind === 'diff' ? tab.path : null
}

// One diff tab per project: opening another file retargets the existing tab wherever it lives.
export function openDiff(project: ProjectRef, path: string): void {
  const layout = workspaces[project.id].layout
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([newDiffTab(path)])
    commit(project, leaf, leaf.id)
    return
  }
  const leaves = tree.allLeaves(root)
  for (const leaf of leaves) {
    const existing = leaf.tabs.find((tab): tab is DiffTab => tab.kind === 'diff')
    if (!existing) continue
    existing.path = path
    commit(project, tree.setActiveTab(root, leaf.id, existing.id), leaf.id)
    return
  }
  const focused = leaves.find((leaf) => leaf.id === layout.focusedPaneId) ?? leaves[0]
  commit(project, tree.addTab(root, focused.id, newDiffTab(path)), focused.id)
}

// One history tab per project: opening it again focuses the existing tab wherever it lives.
export function openHistory(project: ProjectRef): void {
  const layout = workspaces[project.id].layout
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([newHistoryTab()])
    commit(project, leaf, leaf.id)
    return
  }
  const leaves = tree.allLeaves(root)
  for (const leaf of leaves) {
    const existing = leaf.tabs.find((tab) => tab.kind === 'history')
    if (!existing) continue
    commit(project, tree.setActiveTab(root, leaf.id, existing.id), leaf.id)
    return
  }
  const focused = leaves.find((leaf) => leaf.id === layout.focusedPaneId) ?? leaves[0]
  commit(project, tree.addTab(root, focused.id, newHistoryTab()), focused.id)
}

// One vault tab per project: opening it again focuses the existing tab wherever it lives.
export function openVault(project: ProjectRef): void {
  const layout = workspaces[project.id].layout
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([newVaultTab()])
    commit(project, leaf, leaf.id)
    return
  }
  const leaves = tree.allLeaves(root)
  for (const leaf of leaves) {
    const existing = leaf.tabs.find((tab) => tab.kind === 'connector')
    if (!existing) continue
    commit(project, tree.setActiveTab(root, leaf.id, existing.id), leaf.id)
    return
  }
  const focused = leaves.find((leaf) => leaf.id === layout.focusedPaneId) ?? leaves[0]
  commit(project, tree.addTab(root, focused.id, newVaultTab()), focused.id)
}

// One device tab per project: opening it again focuses the existing tab wherever it lives.
export function openDevice(project: ProjectRef): void {
  const layout = workspaces[project.id].layout
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([newDeviceTab()])
    commit(project, leaf, leaf.id)
    return
  }
  const leaves = tree.allLeaves(root)
  for (const leaf of leaves) {
    const existing = leaf.tabs.find((tab) => tab.kind === 'device')
    if (!existing) continue
    commit(project, tree.setActiveTab(root, leaf.id, existing.id), leaf.id)
    return
  }
  const focused = leaves.find((leaf) => leaf.id === layout.focusedPaneId) ?? leaves[0]
  commit(project, tree.addTab(root, focused.id, newDeviceTab()), focused.id)
}

// One tab per workgroup: opening it again focuses the existing tab wherever it lives.
export function openWorkgroup(project: ProjectRef, groupId: number): void {
  const layout = workspaces[project.id].layout
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([newWorkgroupTab(groupId)])
    commit(project, leaf, leaf.id)
    return
  }
  const leaves = tree.allLeaves(root)
  for (const leaf of leaves) {
    const existing = leaf.tabs.find((tab) => tab.kind === 'workgroup' && tab.groupId === groupId)
    if (!existing) continue
    commit(project, tree.setActiveTab(root, leaf.id, existing.id), leaf.id)
    return
  }
  const focused = leaves.find((leaf) => leaf.id === layout.focusedPaneId) ?? leaves[0]
  commit(project, tree.addTab(root, focused.id, newWorkgroupTab(groupId)), focused.id)
}

// One web tab per shell tab: opening it again focuses the existing tab wherever it lives,
// otherwise it opens in a new pane to the right of the shell.
export function openWeb(project: ProjectRef, paneId: string, shellTabId: string): void {
  const root = workspaces[project.id].layout.root
  if (!root) return
  const existing = tree
    .allLeaves(root)
    .find((leaf) => leaf.tabs.some((tab) => tab.kind === 'web' && tab.shellTabId === shellTabId))
  if (existing) {
    const tab = existing.tabs.find((t) => t.kind === 'web' && t.shellTabId === shellTabId)!
    commit(project, tree.setActiveTab(root, existing.id, tab.id), existing.id)
    return
  }
  const tab = newWebTab(shellTabId)
  const next = tree.splitWithTab(root, paneId, 'right', tab)
  commit(project, next, tree.findLeafByTab(next, tab.id)?.id)
}

export function addShell(project: ProjectRef, paneId?: string): void {
  openShellPicker({ kind: 'tab', project, paneId })
}

function addShellWithAgent(
  project: ProjectRef,
  agent: ShellAgent,
  paneId?: string,
  account?: string
): void {
  const layout = workspaces[project.id].layout
  const tab = newShellTab(project, agent, account)
  const root = layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([tab])
    commit(project, leaf, leaf.id)
    focusShellAfterMount(tab.id)
    return
  }
  const leaves = tree.allLeaves(root)
  const target =
    leaves.find((leaf) => leaf.id === paneId) ??
    leaves.find((leaf) => leaf.id === layout.focusedPaneId) ??
    leaves[0]
  commit(project, tree.addTab(root, target.id, tab), target.id)
  focusShellAfterMount(tab.id)
}

export function addShellInNewPane(project: ProjectRef, paneId: string, zone: EdgeZone): void {
  openShellPicker({ kind: 'split', project, paneId, zone })
}

function addShellInNewPaneWithAgent(
  project: ProjectRef,
  paneId: string,
  zone: EdgeZone,
  agent: ShellAgent,
  account?: string
): void {
  const root = workspaces[project.id].layout.root
  if (!root) return
  const tab = newShellTab(project, agent, account)
  const next = tree.splitWithTab(root, paneId, zone, tab)
  commit(project, next, tree.findLeafByTab(next, tab.id)?.id)
  focusShellAfterMount(tab.id)
}

export function openInNewShell(project: ProjectRef, paneId: string, tabId: string): void {
  openShellPicker({ kind: 'openDoc', project, paneId, tabId })
}

function openInNewShellWithAgent(
  project: ProjectRef,
  paneId: string,
  tabId: string,
  agent: ShellAgent,
  account?: string
): void {
  const root = workspaces[project.id].layout.root
  const source = tree.findLeaf(root, paneId)?.tabs.find((tab) => tab.id === tabId)
  if (!root || source?.kind !== 'doc') return
  const tab = newShellTab(project, agent, account)
  // The 'shell' agent spawns zsh directly, which treats a positional prompt argument as a script path to source.
  if (agent.id !== 'shell') {
    setShellArgs(tab.id, [
      `Read ${projectRelativePath(source.path)} and discuss it with the user. Discussing does not replace your usual procedure: orient yourself first, as your instructions say.`
    ])
  }
  const next = tree.splitWithTab(root, paneId, 'right', tab)
  commit(project, next, tree.findLeafByTab(next, tab.id)?.id)
  focusShellAfterMount(tab.id)
}

export function focusPane(project: ProjectRef, paneId: string): void {
  workspaces[project.id].layout.focusedPaneId = paneId
}

export function activateTab(project: ProjectRef, paneId: string, tabId: string): void {
  const root = workspaces[project.id].layout.root
  if (!root) return
  commit(project, tree.setActiveTab(root, paneId, tabId), paneId)
  if (tabsOf(project.id).some((tab) => tab.id === tabId && tab.kind === 'shell')) {
    focusShellAfterMount(tabId)
  }
}

// A workgroup member keeps its saved session so its tab can be reopened with the same identity.
function discardShell(project: ProjectRef, tabId: string): void {
  disposeShell(tabId)
  if (!isWorkgroupMember(project.id, tabId)) void window.api.tabSessions.forget(project.path, tabId)
}

export function closeTab(project: ProjectRef, paneId: string, tabId: string): void {
  const root = workspaces[project.id].layout.root
  if (!root) return
  const tab = tree.findLeaf(root, paneId)?.tabs.find((t) => t.id === tabId)
  if (tab?.kind === 'shell') discardShell(project, tab.id)
  commit(project, tree.closeTab(root, paneId, tabId))
}

export function resizeSplit(project: ProjectRef, splitId: string, sizes: number[]): void {
  const root = workspaces[project.id].layout.root
  if (root) workspaces[project.id].layout.root = tree.setSplitSizes(root, splitId, sizes)
}

export function placeTab(
  project: ProjectRef,
  fromPaneId: string,
  tabId: string,
  toPaneId: string,
  zone: DropZone,
  duplicate: boolean
): void {
  const root = workspaces[project.id].layout.root
  if (!root) return
  const source = tree.findLeaf(root, fromPaneId)?.tabs.find((tab) => tab.id === tabId)
  if (!source || (duplicate && source.kind !== 'doc')) return
  const result = tree.placeTab(root, fromPaneId, tabId, toPaneId, zone, duplicate)
  if (!result) return
  commit(project, result.root, tree.findLeafByTab(result.root, result.placed.id)?.id)
}

function closeMatching(
  project: ProjectRef,
  matches: (tab: WorkspaceTab) => boolean,
  paneId?: string
): void {
  let root = workspaces[project.id].layout.root
  if (!root) return
  for (const leaf of tree.allLeaves(root)) {
    if (paneId && leaf.id !== paneId) continue
    for (const tab of leaf.tabs) {
      if (!root || !matches(tab)) continue
      if (tab.kind === 'shell') discardShell(project, tab.id)
      root = tree.closeTab(root, leaf.id, tab.id)
    }
  }
  commit(project, root)
}

export function closeUnder(project: ProjectRef, prefix: string): void {
  closeMatching(
    project,
    (tab) => tab.kind === 'doc' && (tab.path === prefix || tab.path.startsWith(`${prefix}/`))
  )
}

export function closeAllDocuments(project: ProjectRef): void {
  closeMatching(project, (tab) => tab.kind === 'doc')
}

export function closeAllTabs(project: ProjectRef): void {
  closeMatching(project, () => true)
}

export function closeOtherTabs(project: ProjectRef, paneId: string, keepTabId: string): void {
  closeMatching(project, (tab) => tab.id !== keepTabId, paneId)
}

export function closeTabsInPane(project: ProjectRef, paneId: string): void {
  closeMatching(project, () => true, paneId)
}

export function closeDocumentsInPane(project: ProjectRef, paneId: string): void {
  closeMatching(project, (tab) => tab.kind === 'doc', paneId)
}

onShellAccountChange((tabId, account) => {
  for (const projectId of Object.keys(workspaces)) {
    const tab = tabsOf(projectId).find((candidate) => candidate.id === tabId)
    if (tab?.kind !== 'shell') continue
    if (account) tab.account = account
    else delete tab.account
    return
  }
})

onShellExit((tabId) => {
  for (const [projectId, project] of projectRefs) {
    const root = workspaces[projectId]?.layout.root
    const leaf = tree.findLeafByTab(root ?? null, tabId)
    if (root && leaf) {
      closeTab(project, leaf.id, tabId)
      return
    }
  }
})

const THINKER_ACCOUNTS_KEY = 'lemongrass.thinkerAccounts'

// A closed thinker's tab leaves the layout, so its account is kept here by tab id for a reopen.
function thinkerAccounts(): Record<string, string> {
  try {
    const saved = JSON.parse(localStorage.getItem(THINKER_ACCOUNTS_KEY) ?? '{}')
    return saved && typeof saved === 'object' ? saved : {}
  } catch {
    return {}
  }
}

function rememberThinkerAccounts(entries: Record<string, string>): void {
  try {
    localStorage.setItem(THINKER_ACCOUNTS_KEY, JSON.stringify({ ...thinkerAccounts(), ...entries }))
  } catch {
    return
  }
}

export function shellAccountOf(projectPath: string, tabId: string): string {
  const project = [...projectRefs.values()].find((ref) => ref.path === projectPath)
  const tab = project ? tabsOf(project.id).find((candidate) => candidate.id === tabId) : undefined
  return tab?.kind === 'shell' && tab.account && !accountIsDeleted(tab.account) ? tab.account : ''
}

// A positional prompt starts the session with it as the first message, which extra system prompt text does not.
// Reopens a closed member in the leader's pane under its original tab id, resuming the saved session when there is one.
export async function reopenWorkgroupMember(
  project: ProjectRef,
  member: { tabId: string; label: string; vendor: string },
  leaderTabId: string
): Promise<void> {
  if (isTabOpen(project.id, member.tabId)) return focusTab(project, member.tabId)
  const agent = shellAgentById(member.vendor)
  const sessionId = (await window.api.tabSessions.list(project.path))[member.tabId]
  const args = sessionId ? resumeArgs(agent.command, sessionId) : null
  if (args) setShellResume(member.tabId, args)

  const leaderTab = tabsOf(project.id).find((candidate) => candidate.id === leaderTabId)
  const remembered = thinkerAccounts()[member.tabId]
  const account =
    remembered !== undefined
      ? remembered
      : leaderTab?.kind === 'shell'
        ? leaderTab.account
        : undefined
  const tab: ShellTab = {
    id: member.tabId,
    kind: 'shell',
    label: member.label,
    command: agent.command,
    cwd: project.path,
    thinker: true,
    ...(agent.command === 'claude' && account ? { account } : {})
  }
  const root = workspaces[project.id].layout.root
  if (!root) {
    const leaf = tree.createLeaf<WorkspaceTab>([tab])
    commit(project, leaf, leaf.id)
    return
  }
  const leaves = tree.allLeaves(root)
  const target =
    tree.findLeafByTab(root, leaderTabId) ??
    leaves.find((leaf) => leaf.id === workspaces[project.id].layout.focusedPaneId) ??
    leaves[0]
  commit(
    project,
    tree.setActiveTab(tree.addTab(root, target.id, tab), target.id, tab.id),
    target.id
  )
}

function workgroupArgs(member: WorkgroupSpawnMember): string[] {
  const modelFlag = member.vendor === 'claude' ? '--model' : '-m'
  return [...(member.model ? [modelFlag, member.model] : []), member.prompt]
}

function spawnWorkgroup(request: WorkgroupSpawnRequest): WorkgroupSpawnResult {
  const project = [...projectRefs.values()].find((ref) => ref.path === request.projectPath)
  const root = project ? workspaces[project.id]?.layout.root : null
  const leaderLeaf = tree.findLeafByTab(root ?? null, request.leaderTabId)
  console.info(
    'workgroup: diagnostic leader lookup',
    JSON.stringify({
      requestId: request.requestId,
      projectPath: request.projectPath,
      leaderTabId: request.leaderTabId,
      matchedProjectId: project?.id ?? null,
      matchedProjectPath: project?.path ?? null,
      hasRoot: !!root,
      tabIds: tree.allLeaves(root ?? null).flatMap((leaf) => leaf.tabs.map((tab) => tab.id)),
      leaderPaneId: leaderLeaf?.id ?? null
    })
  )
  if (!project || !root)
    return { ok: false, error: 'the leader project is not open in the workspace' }
  if (!leaderLeaf) return { ok: false, error: 'the leader tab is not in the workspace' }

  const leaderTab = leaderLeaf.tabs.find((tab) => tab.id === request.leaderTabId)
  const leaderAccount = leaderTab?.kind === 'shell' ? leaderTab.account : undefined
  const remembered: Record<string, string> = {}
  const thinkerTabs: ShellTab[] = []
  let next = root
  for (const member of request.members) {
    const account = request.accountsChosen ? member.account : leaderAccount
    if (member.vendor === 'claude') remembered[member.tabId] = account ?? ''
    const tab: ShellTab = {
      id: member.tabId,
      kind: 'shell',
      label: member.label,
      command: shellAgentById(member.vendor).command,
      cwd: project.path,
      thinker: true,
      ...(member.vendor === 'claude' && account ? { account } : {})
    }
    setShellArgs(tab.id, workgroupArgs(member))
    thinkerTabs.push(tab)
    next = tree.addTab(next, leaderLeaf.id, tab)
  }
  rememberThinkerAccounts(remembered)
  commit(project, tree.setActiveTab(next, leaderLeaf.id, request.leaderTabId))
  for (const member of thinkerTabs) {
    startShellDetached({
      id: member.id,
      command: member.command,
      cwd: member.cwd,
      account: member.account,
      thinker: true
    })
  }
  return { ok: true }
}

window.api.workgroup.onSpawn((request) => {
  let result: WorkgroupSpawnResult
  try {
    result = spawnWorkgroup(request)
  } catch (err) {
    result = { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
  console.info(
    'workgroup: diagnostic spawn result',
    JSON.stringify({ requestId: request.requestId, ok: result.ok })
  )
  window.api.workgroup.reportSpawn(request.requestId, result)
})
