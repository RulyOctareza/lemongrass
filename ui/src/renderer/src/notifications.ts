// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type {
  Ask,
  DepsState,
  Notice,
  NoticeInput,
  Project,
  UpdateState,
  WorkgroupPending
} from '../../preload/types'
import * as tree from './layoutTree'
import { askState, openAsk } from './asks'
import { lastSegment, pathWithin } from './pathForm'
import { isWindows } from './platform'
import { askParts, askWhere } from './components/decisions/askOrigin'
import { shellTitles } from './shellRegistry'
import { cleanTitle, tabParts, type NamePart } from './tabNames'
import { vaultSession } from './vaultSession'
import { approvals, openApprovals } from './workgroups'
import {
  focusTab,
  openConflict,
  openVault,
  shellTabById,
  workspaceOf,
  type ProjectRef
} from './workspace'

// The notification center: stored items from main, plus waiting decisions read live, each said as
// one sentence naming the tab as the ask dialog does (tabNames.ts) and the project by its name.

export type SettingsSection = 'updates' | 'dependencies'

export interface CenterHost {
  activeProjectId: () => string | null
  selectProject: (id: string) => Promise<void>
  openSettings: (section: SettingsSection) => void
}

export interface CenterRow {
  key: string
  // Waiting rows are live decisions; the rest are stored items.
  waiting: boolean
  title?: string
  sentence: NamePart[]
  at: number
  unread: boolean
  open: () => void
}

export const center = reactive({
  items: [] as Notice[],
  open: false,
  // Waiting asks read through the vault while its session is unlocked.
  asks: [] as Ask[],
  projects: [] as Project[]
})

let host: CenterHost | null = null
let started = false

export function registerCenterHost(next: CenterHost): void {
  host = next
}

function liveTitle(tabId: string): string {
  const found = shellTabById(tabId)
  return found ? cleanTitle(shellTitles[tabId] ?? '') || found.tab.label : ''
}

function projectOf(path: string | undefined): Project | undefined {
  if (!path) return undefined
  return center.projects
    .filter((p) => pathWithin(p.path, path, isWindows))
    .sort((a, b) => b.path.length - a.path.length)[0]
}

function projectName(path: string | undefined): string {
  if (!path) return 'a project'
  return projectOf(path)?.name ?? lastSegment(path, isWindows)
}

function who(tabId: string | undefined, closedTitle?: string): NamePart[] {
  return tabId ? tabParts(tabId, closedTitle) : [{ text: 'an agent' }]
}

// A group name is model-written like a tab title, so it is marked the same way.
function groupParts(name: string | undefined, adding: boolean): NamePart[] {
  return [
    { text: adding ? 'new thinkers for workgroup ' : 'workgroup ' },
    { text: name ?? '', mark: true }
  ]
}

function line(...parts: (string | NamePart[])[]): NamePart[] {
  return parts.flatMap((part) => (typeof part === 'string' ? [{ text: part }] : part))
}

const ASK_THINGS: Record<string, string> = {
  command: 'a command',
  db: 'a database channel',
  rester: 'a rester channel'
}

const DEP_PROBLEMS: Record<string, string> = {
  missing: 'is missing',
  belowFloor: 'is older than lemongrass needs',
  unknownVersion: 'has a version lemongrass cannot read'
}

// Parts start lower case where a tab name leads; the center renders them capitalized.
export function sentenceOf(item: Notice): NamePart[] {
  const tab = who(item.tabId, item.tabTitle)
  const where = ` in ${projectName(item.projectPath)}`
  switch (item.event) {
    case 'approval-answered': {
      const what = groupParts(item.subject, item.detail === 'add')
      if (item.outcome === 'approved')
        return line('You approved ', what, ' from ', tab, `${where}.`)
      if (item.outcome === 'withdrawn')
        return line(tab, ' withdrew ', what, `${where} before you answered.`)
      return line('You declined ', what, ' from ', tab, `${where}.`)
    }
    case 'ask-answered': {
      const thing = ASK_THINGS[item.detail ?? ''] ?? 'a request'
      if (item.outcome === 'ran') return line(`You ran ${thing} for `, tab, `${where}.`)
      if (item.outcome === 'granted') return line(`You granted ${thing} to `, tab, `${where}.`)
      return line(`You declined ${thing} from `, tab, `${where}.`)
    }
    case 'turn-ended':
      return line(tab, ` finished a ${item.detail}-minute turn${where}.`)
    case 'tab-prompting':
      return line(tab, ` waits for your answer${where}.`)
    case 'conflict-submitted': {
      const count = Number(item.detail) || 1
      const files = count === 1 ? 'one conflicted file' : `${count} conflicted files`
      return line(tab, ` submitted resolutions for ${files}${where}.`)
    }
    case 'update-available':
      return line(
        `lemongrass ${item.subject} is ready to install. You have ${item.detail} installed.`
      )
    case 'dep-problem':
      return line(
        `${item.subject} ${DEP_PROBLEMS[item.detail ?? ''] ?? 'needs attention'}, and lemongrass needs it.`
      )
    case 'vault-locked':
      return line('The vault locked after 5 minutes without use.')
    case 'git-failed': {
      const action = item.detail === 'push' ? 'Push' : 'Fetch'
      return line(
        item.subject
          ? `${action} failed${where}. Git said: ${item.subject}`
          : `${action} failed${where}.`
      )
    }
    case 'popup':
      return line(item.detail ?? '')
  }
}

async function showProject(path: string | undefined): Promise<ProjectRef | null> {
  const project = projectOf(path)
  if (!project || !host) return null
  if (host.activeProjectId() !== project.id) await host.selectProject(project.id)
  return { id: project.id, path: project.path }
}

async function goToTab(item: Notice): Promise<void> {
  const found = item.tabId ? shellTabById(item.tabId) : null
  const project = await showProject(found?.project.path ?? item.projectPath)
  if (project && item.tabId && found) focusTab(project, item.tabId)
}

async function goTo(item: Notice): Promise<void> {
  center.open = false
  switch (item.event) {
    case 'update-available':
      host?.openSettings('updates')
      return
    case 'dep-problem':
      host?.openSettings('dependencies')
      return
    case 'vault-locked': {
      const id = host?.activeProjectId()
      const project = center.projects.find((p) => p.id === id)
      if (project) openVault({ id: project.id, path: project.path })
      return
    }
    case 'git-failed':
      await showProject(item.projectPath)
      return
    case 'conflict-submitted': {
      const project = await showProject(item.projectPath)
      if (project && item.subject) openConflict(project, item.subject)
      return
    }
    case 'popup':
      return
    default:
      await goToTab(item)
  }
}

function storedRow(item: Notice): CenterRow {
  return {
    key: item.id,
    waiting: false,
    title: item.event === 'popup' ? item.subject : undefined,
    sentence: sentenceOf(item),
    at: item.createdAt,
    unread: !item.read,
    open: () => {
      if (!item.read) void window.api.notifications.markRead([item.id])
      void goTo(item)
    }
  }
}

function approvalRow(request: WorkgroupPending): CenterRow {
  return {
    key: `approval:${request.requestId}`,
    waiting: true,
    sentence: line(
      who(request.leaderTabId),
      ' proposes ',
      groupParts(request.groupName, !!request.existingMembers),
      ` in ${projectName(request.projectPath)} and waits for your approval.`
    ),
    at: request.createdAt,
    unread: true,
    open: () => {
      center.open = false
      openApprovals(request.requestId)
    }
  }
}

// The dialog's headline leaves the project out; the center spans projects, so it goes before the full stop.
function withProject(parts: NamePart[], where: string): NamePart[] {
  const last = parts[parts.length - 1]
  if (!last || last.mark) return [...parts, { text: ` in ${where}.` }]
  return [...parts.slice(0, -1), { text: `${last.text.replace(/\.$/, '')} in ${where}.` }]
}

function askRow(ask: Ask): CenterRow {
  return {
    key: `ask:${ask.id}`,
    waiting: true,
    sentence: withProject(askParts(ask), askWhere(ask)),
    at: new Date(ask.created_at).getTime(),
    unread: true,
    open: () => {
      center.open = false
      openAsk(ask.id)
    }
  }
}

// While the vault is locked only the count is readable, so the asks show as one row.
function lockedAsksRow(waiting: number): CenterRow {
  return {
    key: 'asks:locked',
    waiting: true,
    sentence: line(
      waiting === 1
        ? 'An agent waits for your decision. Unlock the vault to read and answer it.'
        : `${waiting} agent requests wait for your decision. Unlock the vault to read and answer them.`
    ),
    at: Date.now(),
    unread: true,
    open: () => {
      center.open = false
      openAsk('')
    }
  }
}

export function waitingRows(): CenterRow[] {
  const rows = approvals.pending.map(approvalRow)
  const asks = center.asks.filter((ask) => ask.state === 'waiting')
  if (vaultSession.unlocked && asks.length > 0) rows.push(...asks.map(askRow))
  else if (askState.count.waiting > 0) rows.push(lockedAsksRow(askState.count.waiting))
  return rows.sort((a, b) => b.at - a.at)
}

export function storedRows(): CenterRow[] {
  return center.items.map(storedRow)
}

export function unreadCount(): number {
  return (
    center.items.filter((item) => !item.read).length +
    askState.count.waiting +
    approvals.pending.length
  )
}

export function loadProjects(): void {
  window.api.projects
    .list()
    .then((list) => (center.projects = list))
    .catch(() => undefined)
}

export async function loadWaitingAsks(): Promise<void> {
  if (!vaultSession.unlocked) {
    center.asks = []
    return
  }
  try {
    center.asks = await window.api.ask.list(vaultSession.passphrase)
  } catch {
    center.asks = []
  }
}

function add(input: NoticeInput): void {
  void window.api.notifications.add(input).catch(() => undefined)
}

// Main pushes state on every change and again at each start, so these are recorded once per key.
function addOnce(input: NoticeInput): void {
  add({ ...input, once: true })
}

export function noteAskAnswered(ask: Ask, outcome: 'ran' | 'declined' | 'granted'): void {
  const found = shellTabById(ask.tab_id)
  add({
    event: 'ask-answered',
    projectPath: found?.project.path ?? projectOf(ask.cwd)?.path ?? ask.cwd,
    tabId: ask.tab_id,
    tabTitle: liveTitle(ask.tab_id) || undefined,
    detail: ask.type,
    outcome
  })
}

function onUpdate(state: UpdateState): void {
  if (!state.available || !state.latest) return
  addOnce({
    event: 'update-available',
    key: `update:${state.latest}`,
    subject: state.latest,
    detail: state.current
  })
}

function onDeps(state: DepsState): void {
  for (const dep of state.report?.deps ?? []) {
    if (!dep.required || dep.status === 'ok') continue
    addOnce({
      event: 'dep-problem',
      key: `dep:${dep.name}:${dep.status}`,
      subject: dep.name,
      detail: dep.status
    })
  }
}

const POLL_MS = 10_000
const LONG_TURN_MS = 3 * 60 * 1000

interface TabWatch {
  state: 'working' | 'prompting' | 'idle'
  since: number
}

const watched = new Map<string, TabWatch>()

// tabSessions:states reports these reasons for a busy tab; anything else is idle.
function stateOf(reason: string | null | undefined): TabWatch['state'] {
  if (reason === 'Mid-turn') return 'working'
  if (reason === 'Permission prompt open') return 'prompting'
  return 'idle'
}

function shellTabsOf(project: Project): { tabId: string; shown: boolean }[] {
  const root = workspaceOf(project.id)?.layout.root ?? null
  const active = host?.activeProjectId() === project.id
  return tree
    .allLeaves(root)
    .flatMap((leaf) =>
      leaf.tabs
        .filter((tab) => tab.kind === 'shell')
        .map((tab) => ({ tabId: tab.id, shown: active && leaf.activeTabId === tab.id }))
    )
}

function noteTab(
  event: 'turn-ended' | 'tab-prompting',
  project: Project,
  tabId: string,
  shown: boolean,
  detail?: string
): void {
  add({
    event,
    key: `${event}:${tabId}`,
    projectPath: project.path,
    tabId,
    tabTitle: liveTitle(tabId) || undefined,
    detail,
    read: shown && document.hasFocus()
  })
}

async function pollTabs(): Promise<void> {
  const now = Date.now()
  const seen = new Set<string>()
  for (const project of center.projects) {
    const tabs = shellTabsOf(project)
    if (tabs.length === 0) continue
    let states: Record<string, { reason: string | null }>
    try {
      states = await window.api.tabSessions.states(project.path)
    } catch {
      continue
    }
    for (const { tabId, shown } of tabs) {
      seen.add(tabId)
      const next = stateOf(states[tabId]?.reason)
      const before = watched.get(tabId)
      if (!before) {
        watched.set(tabId, { state: next, since: now })
        continue
      }
      if (before.state === next) continue
      if (before.state !== 'idle' && next === 'idle' && now - before.since >= LONG_TURN_MS) {
        const minutes = Math.round((now - before.since) / 60_000)
        noteTab('turn-ended', project, tabId, shown, String(minutes))
      }
      if (next === 'prompting') noteTab('tab-prompting', project, tabId, shown)
      // A turn paused on a prompt is still the same turn, so its start time carries over.
      const keepsTurn = before.state !== 'idle' && next !== 'idle'
      watched.set(tabId, { state: next, since: keepsTurn ? before.since : now })
    }
  }
  for (const tabId of watched.keys()) if (!seen.has(tabId)) watched.delete(tabId)
}

export function startNotificationFeed(): void {
  if (started) return
  started = true
  loadProjects()
  window.api.notifications
    .list()
    .then((items) => (center.items = items))
    .catch(() => undefined)
  window.api.notifications.onChanged((items) => (center.items = items))
  window.api.update
    .state()
    .then(onUpdate)
    .catch(() => undefined)
  window.api.update.onState(onUpdate)
  window.api.deps
    .state()
    .then(onDeps)
    .catch(() => undefined)
  window.api.deps.onState(onDeps)
  window.api.conflictAgent.onSubmitted((submitted) =>
    add({
      event: 'conflict-submitted',
      projectPath: submitted.projectPath,
      tabId: submitted.tabId,
      tabTitle: liveTitle(submitted.tabId) || undefined,
      subject: submitted.files[0],
      detail: String(submitted.files.length)
    })
  )
  setInterval(() => void pollTabs(), POLL_MS)
}
