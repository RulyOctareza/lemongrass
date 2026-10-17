import { reactive } from 'vue'
import type { WorkgroupInfo, WorkgroupPending, WorkspaceTab } from '../../preload/types'
import type { ProjectRef } from './workspace'
import { accountStore } from './accounts'

export interface WorkgroupState {
  groups: WorkgroupInfo[]
  names: Record<number, string>
  folded: boolean
  confirming: number | null
  busy: boolean
  error: string | null
}

const states = reactive<Record<string, WorkgroupState>>({})
const refreshing = new Set<string>()

export function workgroupStateOf(projectId: string): WorkgroupState {
  if (!states[projectId]) {
    states[projectId] = {
      groups: [],
      names: {},
      folded: true,
      confirming: null,
      busy: false,
      error: null
    }
  }
  return states[projectId]
}

export async function refreshWorkgroups(project: ProjectRef): Promise<void> {
  if (refreshing.has(project.id)) return
  refreshing.add(project.id)
  try {
    const next = await window.api.workgroup.list(project.path)
    const state = workgroupStateOf(project.id)
    if (JSON.stringify(state.groups) !== JSON.stringify(next)) state.groups = next
    for (const group of next) state.names[group.id] = group.name
    if (state.confirming !== null && !next.some((group) => group.id === state.confirming)) {
      state.confirming = null
    }
  } finally {
    refreshing.delete(project.id)
  }
}

export async function disbandWorkgroup(project: ProjectRef, groupId: number): Promise<void> {
  const state = workgroupStateOf(project.id)
  state.busy = true
  state.error = null
  try {
    const result = await window.api.workgroup.disband(project.path, groupId)
    if (!result.ok) state.error = result.error ?? 'Could not disband the workgroup.'
    state.confirming = null
    await refreshWorkgroups(project)
  } finally {
    state.busy = false
  }
}

export const approvals = reactive<{
  pending: WorkgroupPending[]
  open: boolean
  selectedId: string | null
  busy: boolean
  reasons: Record<string, string>
  declining: Record<string, boolean>
  accounts: Record<string, string[]>
}>({
  pending: [],
  open: false,
  selectedId: null,
  busy: false,
  reasons: {},
  declining: {},
  accounts: {}
})

export function isWorkgroupMember(projectId: string, tabId: string): boolean {
  return (states[projectId]?.groups ?? []).some((group) =>
    group.members.some((member) => member.tabId === tabId)
  )
}

export function tabGroup(
  projectId: string,
  tab: WorkspaceTab
): { id: number; leader: boolean } | null {
  if (tab.kind === 'workgroup') return { id: tab.groupId, leader: false }
  if (tab.kind !== 'shell') return null
  const group = (states[projectId]?.groups ?? []).find((g) =>
    g.members.some((member) => member.tabId === tab.id)
  )
  return group ? { id: group.id, leader: group.leaderTabId === tab.id } : null
}

export function pendingFor(projectPath: string): WorkgroupPending[] {
  return approvals.pending.filter((request) => request.projectPath === projectPath)
}

function setPending(next: WorkgroupPending[]): void {
  approvals.pending = next
  for (const requestId of Object.keys(approvals.reasons)) {
    if (!next.some((request) => request.requestId === requestId))
      delete approvals.reasons[requestId]
  }
  for (const requestId of Object.keys(approvals.declining)) {
    if (!next.some((request) => request.requestId === requestId))
      delete approvals.declining[requestId]
  }
  for (const requestId of Object.keys(approvals.accounts)) {
    if (!next.some((request) => request.requestId === requestId))
      delete approvals.accounts[requestId]
  }
  if (next.length === 0) approvals.open = false
  if (!next.some((request) => request.requestId === approvals.selectedId)) {
    approvals.selectedId = next[0]?.requestId ?? null
  }
}

export function openApprovals(requestId?: string): void {
  if (approvals.pending.length === 0) return
  if (requestId) approvals.selectedId = requestId
  approvals.open = true
}

export function closeApprovals(): void {
  approvals.open = false
}

// Starts every Claude thinker on the leader's account ('' for primary) the first time a request is shown.
export function ensureAccountChoices(request: WorkgroupPending, leaderAccount: string): void {
  if (approvals.accounts[request.requestId]) return
  approvals.accounts[request.requestId] = request.members.map((member) =>
    member.vendor === 'claude' ? leaderAccount : ''
  )
}

export async function decideApproval(requestId: string, approved: boolean): Promise<void> {
  approvals.busy = true
  try {
    const reason = approved ? undefined : approvals.reasons[requestId]
    const choices = approvals.accounts[requestId]
    const accounts = approved && choices && accountStore.items.length > 0 ? [...choices] : undefined
    await window.api.workgroup.decide(requestId, approved, reason, accounts)
  } finally {
    approvals.busy = false
  }
}

window.api.workgroup.onPendingChange(setPending)
window.api.workgroup.onOpenApprovals(() => openApprovals())
void window.api.workgroup.pending().then(setPending)

export interface JoinDraft {
  project: ProjectRef
  groupId: number
  groupName: string
  tabId: string
  tabTitle: string
  vendor: string
  label: string
}

// notice is set once the tab has joined but something after the join fell short, so the modal offers only Close.
export const joining = reactive<{
  draft: JoinDraft | null
  busy: boolean
  error: string | null
  notice: string | null
}>({ draft: null, busy: false, error: null, notice: null })

const JOINABLE_COMMANDS: Record<string, string> = { claude: 'claude', codex: 'codex' }
const LABEL_MAX = 30

// A Claude or Codex shell in no live group can be added; Cursor and plain shells have no hooks, so no gate would hold them.
export function joinVendor(projectId: string, tab: WorkspaceTab): string | null {
  if (tab.kind !== 'shell' || isWorkgroupMember(projectId, tab.id)) return null
  return JOINABLE_COMMANDS[tab.command] ?? null
}

export function liveGroupsOf(projectId: string): WorkgroupInfo[] {
  return states[projectId]?.groups ?? []
}

// The tab title reduced to the member label pattern and made unique in the group, falling back to the vendor name.
export function joinLabel(title: string, vendor: string, group: WorkgroupInfo): string {
  const cleaned = title
    .replace(/[^A-Za-z0-9_-]+/g, '-')
    .replace(/^[-_]+|[-_]+$/g, '')
    .slice(0, LABEL_MAX)
  const base = /^[A-Za-z0-9]/.test(cleaned) ? cleaned : vendor
  const taken = new Set(group.members.map((member) => member.label.toLowerCase()))
  if (!taken.has(base.toLowerCase())) return base
  for (let n = 2; ; n++) {
    const suffix = `-${n}`
    const candidate = base.slice(0, LABEL_MAX - suffix.length) + suffix
    if (!taken.has(candidate.toLowerCase())) return candidate
  }
}

export function openJoin(
  project: ProjectRef,
  group: WorkgroupInfo,
  tab: WorkspaceTab,
  tabTitle: string
): void {
  const vendor = joinVendor(project.id, tab)
  if (!vendor) return
  joining.error = null
  joining.notice = null
  joining.draft = {
    project,
    groupId: group.id,
    groupName: group.name,
    tabId: tab.id,
    tabTitle,
    vendor,
    label: joinLabel(tabTitle, vendor, group)
  }
}

export function closeJoin(): void {
  if (joining.busy) return
  joining.draft = null
  joining.error = null
  joining.notice = null
}

export async function confirmJoin(): Promise<void> {
  const draft = joining.draft
  if (!draft || joining.busy) return
  joining.busy = true
  joining.error = null
  try {
    const result = await window.api.workgroup.join({
      projectPath: draft.project.path,
      groupId: draft.groupId,
      tabId: draft.tabId,
      label: draft.label
    })
    if (!result.ok) {
      joining.error = result.error ?? 'The tab could not join the workgroup.'
      return
    }
    await refreshWorkgroups(draft.project)
    if (result.warning) {
      joining.notice = result.warning
      return
    }
    joining.draft = null
  } finally {
    joining.busy = false
  }
}
