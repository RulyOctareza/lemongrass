// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { contextBridge, ipcRenderer } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'
import type {
  AccountAddResult,
  Ask,
  AskCount,
  AccountInfo,
  AccountRemoveResult,
  ContextUsage,
  UsageReport,
  StatusLineChainState,
  UpdateState,
  DepsState,
  CreditsResult,
  DeviceEnd,
  DeviceListResult,
  DevicePacket,
  DeviceStartRequest,
  DeviceStartResult,
  PtySpawnOptions,
  ResolvedSSHHost,
  PtyDataPayload,
  PtyExitPayload,
  Project,
  UiState,
  BiblioTree,
  BiblioFileResult,
  ScratchpadImageResult,
  ImageFileResult,
  GitStatus,
  GitDiff,
  GitActionResult,
  GitMoveResult,
  GitBlame,
  GitBranches,
  GitCommitInfo,
  GitLogResult,
  GitLogSearchMode,
  GitResetPreview,
  GitDeleteResult,
  GitPushResult,
  GitUpdateResult,
  GitForecast,
  GitRevFiles,
  GitRevision,
  GitStash,
  GitConflictChoice,
  GitConflictFile,
  ConflictActionResult,
  ConflictHumanMessage,
  ConflictReviewFile,
  ConflictStartResult,
  ConflictSubmitted,
  ConflictThreadStatus,
  ReviewSubmission,
  ReviewSubmitResult,
  TabBusy,
  NodeIssued,
  ResterClientCall,
  ResterClientDownload,
  ResterClientRequest,
  ResterClientResult,
  ResterClientRun,
  NodeIssueRequest,
  NodeState,
  NodeUpdateRequest,
  VaultScope,
  VaultChannel,
  VaultChannelWithShortId,
  WebBounds,
  WebState,
  VaultRester,
  VaultResterUser,
  VaultResterLoginTest,
  VaultEnvironment,
  VaultResterAPI,
  VaultWebScope,
  VaultHTTPScope,
  VaultHTTPChannel,
  VaultHTTPChannelWithShortId,
  VaultPolicies,
  VaultPolicy,
  VaultPolicyRule,
  WorkspaceLayoutState,
  WorkgroupDisbandResult,
  WorkgroupJoinRequest,
  WorkgroupJoinResult,
  WorkgroupInfo,
  WorkgroupPending,
  WorkgroupSpawnRequest,
  WorkgroupSpawnResult,
  WorkgroupThreadResult
} from './types'

// Custom APIs for renderer -- raw PTY bytes only, never a control-signal
// channel. PTY is display-only.
const pty = {
  spawn: (opts: PtySpawnOptions): Promise<{ id: string }> => ipcRenderer.invoke('pty:spawn', opts),
  write: (id: string, data: string): void => ipcRenderer.send('pty:write', { id, data }),
  resize: (id: string, cols: number, rows: number): void =>
    ipcRenderer.send('pty:resize', { id, cols, rows }),
  kill: (id: string): void => ipcRenderer.send('pty:kill', { id }),
  gracefulClose: (id: string): void => ipcRenderer.send('pty:graceful-close', { id }),
  onData: (callback: (payload: PtyDataPayload) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, payload: PtyDataPayload): void =>
      callback(payload)
    ipcRenderer.on('pty:data', listener)
    return () => ipcRenderer.removeListener('pty:data', listener)
  },
  onExit: (callback: (payload: PtyExitPayload) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, payload: PtyExitPayload): void =>
      callback(payload)
    ipcRenderer.on('pty:exit', listener)
    return () => ipcRenderer.removeListener('pty:exit', listener)
  }
}

const tabSessions = {
  list: (projectPath: string): Promise<Record<string, string>> =>
    ipcRenderer.invoke('tabSessions:list', projectPath),
  clear: (projectPath: string): Promise<void> =>
    ipcRenderer.invoke('tabSessions:clear', projectPath),
  title: (projectPath: string, tabId: string, title: string): Promise<void> =>
    ipcRenderer.invoke('tabSessions:title', { projectPath, tabId, title }),
  forget: (projectPath: string, tabId: string): Promise<void> =>
    ipcRenderer.invoke('tabSessions:forget', { projectPath, tabId }),
  states: (projectPath: string): Promise<Record<string, TabBusy>> =>
    ipcRenderer.invoke('tabSessions:states', projectPath)
}

const review = {
  submit: (
    projectPath: string,
    tabId: string,
    submission: ReviewSubmission
  ): Promise<ReviewSubmitResult> =>
    ipcRenderer.invoke('review:submit', projectPath, tabId, submission)
}

// Handing a conflict to an agent tab. resolved is what saveConflictWork returns, so main can tell
// the model which blocks the human already resolved.
const conflictAgent = {
  status: (projectPath: string): Promise<ConflictThreadStatus | null> =>
    ipcRenderer.invoke('conflict:status', projectPath),
  start: (
    projectPath: string,
    tabId: string,
    resolved: ConflictReviewFile[]
  ): Promise<ConflictStartResult> =>
    ipcRenderer.invoke('conflict:start', projectPath, tabId, resolved),
  post: (
    projectPath: string,
    messages: ConflictHumanMessage[],
    resolved: ConflictReviewFile[]
  ): Promise<ConflictActionResult> =>
    ipcRenderer.invoke('conflict:post', projectPath, messages, resolved),
  close: (projectPath: string): Promise<ConflictActionResult> =>
    ipcRenderer.invoke('conflict:close', projectPath),
  onSubmitted: (callback: (submitted: ConflictSubmitted) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, submitted: ConflictSubmitted): void =>
      callback(submitted)
    ipcRenderer.on('conflict:submitted', listener)
    return () => ipcRenderer.removeListener('conflict:submitted', listener)
  }
}

const projects = {
  list: (): Promise<Project[]> => ipcRenderer.invoke('projects:list'),
  add: (): Promise<Project | null> => ipcRenderer.invoke('projects:add'),
  setTrust: (id: string, trusted: boolean): Promise<Project | null> =>
    ipcRenderer.invoke('projects:setTrust', id, trusted)
}

const workspaceLayouts = {
  load: (projectId: string): Promise<WorkspaceLayoutState | null> =>
    ipcRenderer.invoke('workspaceLayouts:load', projectId),
  save: (projectId: string, layout: WorkspaceLayoutState | null): void =>
    ipcRenderer.send('workspaceLayouts:save', { projectId, layout })
}

const uiState = {
  load: (): Promise<UiState> => ipcRenderer.invoke('uiState:load'),
  save: (patch: UiState): void => ipcRenderer.send('uiState:save', patch),
  zoomPresets: (): Promise<number[]> => ipcRenderer.invoke('uiState:zoomPresets')
}

const biblio = {
  tree: (projectPath: string): Promise<BiblioTree | null> =>
    ipcRenderer.invoke('biblio:tree', projectPath),
  read: (projectPath: string, relativePath: string): Promise<string | null> =>
    ipcRenderer.invoke('biblio:read', projectPath, relativePath),
  readFile: (projectPath: string, relativePath: string): Promise<BiblioFileResult | null> =>
    ipcRenderer.invoke('biblio:readFile', projectPath, relativePath),
  write: (projectPath: string, relativePath: string, content: string): Promise<boolean> =>
    ipcRenderer.invoke('biblio:write', projectPath, relativePath, content),
  createScratchpad: (projectPath: string, title: string, content: string): Promise<string | null> =>
    ipcRenderer.invoke('biblio:createScratchpad', projectPath, title, content),
  createScratchpadFile: (
    projectPath: string,
    folderPath: string,
    title: string,
    content: string
  ): Promise<string | null> =>
    ipcRenderer.invoke('biblio:createScratchpadFile', projectPath, folderPath, title, content),
  saveScratchpadImage: (
    projectPath: string,
    notePath: string,
    bytes: Uint8Array,
    mime: string
  ): Promise<ScratchpadImageResult> =>
    ipcRenderer.invoke('biblio:saveScratchpadImage', projectPath, notePath, bytes, mime),
  readImageFile: (filePath: string): Promise<ImageFileResult> =>
    ipcRenderer.invoke('biblio:readImageFile', filePath),
  archiveScratchpad: (projectPath: string, relativePath: string): Promise<boolean> =>
    ipcRenderer.invoke('biblio:archiveScratchpad', projectPath, relativePath),
  onChanged: (callback: (projectPath: string) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, projectPath: string): void =>
      callback(projectPath)
    ipcRenderer.on('biblio:changed', listener)
    return () => ipcRenderer.removeListener('biblio:changed', listener)
  }
}

const git = {
  status: (projectPath: string): Promise<GitStatus> =>
    ipcRenderer.invoke('git:status', projectPath),
  diff: (projectPath: string, path: string, origPath?: string): Promise<GitDiff> =>
    ipcRenderer.invoke('git:diff', projectPath, path, origPath),
  commit: (projectPath: string, message: string, paths?: string[]): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:commit', projectPath, message, paths),
  push: (projectPath: string): Promise<GitPushResult> =>
    ipcRenderer.invoke('git:push', projectPath),
  lastMessage: (projectPath: string): Promise<string> =>
    ipcRenderer.invoke('git:lastMessage', projectPath),
  branches: (projectPath: string): Promise<GitBranches> =>
    ipcRenderer.invoke('git:branches', projectPath),
  switchBranch: (projectPath: string, name: string, remote: boolean): Promise<GitMoveResult> =>
    ipcRenderer.invoke('git:switchBranch', projectPath, name, remote),
  createBranch: (projectPath: string, name: string, base: string): Promise<GitMoveResult> =>
    ipcRenderer.invoke('git:createBranch', projectPath, name, base),
  branchNameValid: (projectPath: string, name: string): Promise<boolean> =>
    ipcRenderer.invoke('git:branchNameValid', projectPath, name),
  updateBranch: (projectPath: string, name: string, remote: boolean): Promise<GitUpdateResult> =>
    ipcRenderer.invoke('git:updateBranch', projectPath, name, remote),
  rebaseOntoUpstream: (projectPath: string, name: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:rebaseOntoUpstream', projectPath, name),
  rebaseForecast: (projectPath: string, name: string): Promise<GitForecast> =>
    ipcRenderer.invoke('git:rebaseForecast', projectPath, name),
  deleteBranch: (projectPath: string, name: string): Promise<GitDeleteResult> =>
    ipcRenderer.invoke('git:deleteBranch', projectPath, name),
  commitsOnlyOn: (projectPath: string, name: string): Promise<number | null> =>
    ipcRenderer.invoke('git:commitsOnlyOn', projectPath, name),
  revFiles: (projectPath: string, rev: GitRevision): Promise<GitRevFiles> =>
    ipcRenderer.invoke('git:revFiles', projectPath, rev),
  revDiff: (
    projectPath: string,
    rev: GitRevision,
    path: string,
    origPath?: string
  ): Promise<GitDiff> => ipcRenderer.invoke('git:revDiff', projectPath, rev, path, origPath),
  stashList: (projectPath: string): Promise<GitStash[]> =>
    ipcRenderer.invoke('git:stashList', projectPath),
  stashPush: (
    projectPath: string,
    message: string,
    includeUntracked: boolean
  ): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:stashPush', projectPath, message, includeUntracked),
  stashAction: (
    projectPath: string,
    ref: string,
    action: 'apply' | 'pop' | 'drop'
  ): Promise<GitActionResult> => ipcRenderer.invoke('git:stashAction', projectPath, ref, action),
  fetch: (projectPath: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:fetch', projectPath),
  mergeCount: (projectPath: string, source: string): Promise<number> =>
    ipcRenderer.invoke('git:mergeCount', projectPath, source),
  mergeForecast: (projectPath: string, source: string): Promise<GitForecast> =>
    ipcRenderer.invoke('git:mergeForecast', projectPath, source),
  merge: (projectPath: string, source: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:merge', projectPath, source),
  finish: (projectPath: string, action: 'continue' | 'abort'): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:finish', projectPath, action),
  conflict: (projectPath: string, path: string): Promise<GitConflictFile> =>
    ipcRenderer.invoke('git:conflict', projectPath, path),
  conflictApply: (projectPath: string, path: string, content: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:conflictApply', projectPath, path, content),
  conflictSave: (projectPath: string, path: string, content: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:conflictSave', projectPath, path, content),
  conflictPick: (
    projectPath: string,
    path: string,
    choice: GitConflictChoice
  ): Promise<GitActionResult> => ipcRenderer.invoke('git:conflictPick', projectPath, path, choice),
  log: (
    projectPath: string,
    mode: GitLogSearchMode,
    text: string,
    skip: number,
    limit: number
  ): Promise<GitLogResult> => ipcRenderer.invoke('git:log', projectPath, mode, text, skip, limit),
  logToken: (projectPath: string): Promise<string> =>
    ipcRenderer.invoke('git:logToken', projectPath),
  commitInfo: (projectPath: string, hash: string): Promise<GitCommitInfo | null> =>
    ipcRenderer.invoke('git:commitInfo', projectPath, hash),
  blame: (projectPath: string, hash: string, path: string): Promise<GitBlame> =>
    ipcRenderer.invoke('git:blame', projectPath, hash, path),
  resetPreview: (projectPath: string, hash: string): Promise<GitResetPreview> =>
    ipcRenderer.invoke('git:resetPreview', projectPath, hash),
  resetSoft: (projectPath: string, hash: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:resetSoft', projectPath, hash),
  amendMessage: (projectPath: string, message: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:amendMessage', projectPath, message)
}

const device = {
  list: (): Promise<DeviceListResult> => ipcRenderer.invoke('device:list'),
  start: (request: DeviceStartRequest): Promise<DeviceStartResult> =>
    ipcRenderer.invoke('device:start', request),
  stop: (id: string): void => ipcRenderer.send('device:stop', id),
  control: (id: string, message: Uint8Array): void =>
    ipcRenderer.send('device:control', id, message),
  onPacket: (callback: (packet: DevicePacket) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, packet: DevicePacket): void =>
      callback(packet)
    ipcRenderer.on('device:packet', listener)
    return () => ipcRenderer.removeListener('device:packet', listener)
  },
  onEnd: (callback: (end: DeviceEnd) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, end: DeviceEnd): void => callback(end)
    ipcRenderer.on('device:end', listener)
    return () => ipcRenderer.removeListener('device:end', listener)
  }
}

const resterClient = {
  request: (passphrase: string, call: ResterClientCall): Promise<ResterClientResult> =>
    ipcRenderer.invoke('resterClient:request', passphrase, call),
  download: (passphrase: string, call: ResterClientCall): Promise<ResterClientDownload> =>
    ipcRenderer.invoke('resterClient:download', passphrase, call),
  flush: (rester: string, environment: string, user: string): Promise<number> =>
    ipcRenderer.invoke('resterClient:flush', rester, environment, user),
  load: (): Promise<ResterClientRequest[]> => ipcRenderer.invoke('resterClient:load'),
  save: (requests: ResterClientRequest[]): Promise<void> =>
    ipcRenderer.invoke('resterClient:save', requests),
  history: (requestId: string): Promise<ResterClientRun[]> =>
    ipcRenderer.invoke('resterClient:history', requestId),
  recordRun: (requestId: string, run: ResterClientRun): Promise<ResterClientRun[]> =>
    ipcRenderer.invoke('resterClient:recordRun', requestId, run),
  deleteRun: (requestId: string, runId: string): Promise<ResterClientRun[]> =>
    ipcRenderer.invoke('resterClient:deleteRun', requestId, runId),
  clearHistory: (requestId: string): Promise<void> =>
    ipcRenderer.invoke('resterClient:clearHistory', requestId)
}

const node = {
  state: (): Promise<NodeState> => ipcRenderer.invoke('node:state'),
  saveTunnel: (
    provider: 'ngrok' | 'local',
    authtoken: string | null,
    domain: string,
    localPort: number
  ): Promise<NodeState> =>
    ipcRenderer.invoke('node:saveTunnel', provider, authtoken, domain, localPort),
  openTunnel: (hours: number): Promise<NodeState> => ipcRenderer.invoke('node:openTunnel', hours),
  closeTunnel: (): Promise<NodeState> => ipcRenderer.invoke('node:closeTunnel'),
  issue: (req: NodeIssueRequest): Promise<NodeIssued> => ipcRenderer.invoke('node:issue', req),
  update: (req: NodeUpdateRequest): Promise<NodeState> => ipcRenderer.invoke('node:update', req),
  revoke: (id: string): Promise<NodeState> => ipcRenderer.invoke('node:revoke', id),
  remove: (id: string): Promise<NodeState> => ipcRenderer.invoke('node:remove', id)
}

const vault = {
  list: (): Promise<VaultChannel[]> => ipcRenderer.invoke('vault:list'),
  create: (
    passphrase: string,
    name: string,
    dbName: string,
    scope: VaultScope,
    ttlSeconds: number,
    askId?: string
  ): Promise<VaultChannelWithShortId> =>
    ipcRenderer.invoke('vault:create', passphrase, name, dbName, scope, ttlSeconds, askId),
  activate: (
    passphrase: string,
    id: string,
    ttlSeconds: number
  ): Promise<VaultChannelWithShortId> =>
    ipcRenderer.invoke('vault:activate', passphrase, id, ttlSeconds),
  revoke: (id: string): Promise<void> => ipcRenderer.invoke('vault:revoke', id),
  listConnections: (): Promise<string[]> => ipcRenderer.invoke('vault:listConnections'),
  putCredential: (passphrase: string, name: string, connectionString: string): Promise<void> =>
    ipcRenderer.invoke('vault:putCredential', passphrase, name, connectionString),
  getConnection: (passphrase: string, name: string): Promise<string> =>
    ipcRenderer.invoke('vault:getConnection', passphrase, name),
  updateConnection: (passphrase: string, name: string, connectionString: string): Promise<void> =>
    ipcRenderer.invoke('vault:updateConnection', passphrase, name, connectionString),
  deleteConnection: (name: string): Promise<void> =>
    ipcRenderer.invoke('vault:deleteConnection', name),
  testConnection: (connectionString: string): Promise<void> =>
    ipcRenderer.invoke('vault:testConnection', connectionString),
  testSavedConnection: (passphrase: string, name: string): Promise<void> =>
    ipcRenderer.invoke('vault:testSavedConnection', passphrase, name),
  listTables: (passphrase: string, name: string): Promise<string[]> =>
    ipcRenderer.invoke('vault:listTables', passphrase, name),
  connectionSSHHosts: (passphrase: string): Promise<Record<string, string>> =>
    ipcRenderer.invoke('vault:connectionSSHHosts', passphrase),
  listSSHHosts: (): Promise<string[]> => ipcRenderer.invoke('vault:listSSHHosts'),
  resolveSSHHost: (alias: string): Promise<ResolvedSSHHost> =>
    ipcRenderer.invoke('vault:resolveSSHHost', alias),
  hasPassphrase: (): Promise<boolean> => ipcRenderer.invoke('vault:hasPassphrase'),
  setPassphrase: (passphrase: string): Promise<void> =>
    ipcRenderer.invoke('vault:setPassphrase', passphrase),
  verifyPassphrase: (passphrase: string): Promise<void> =>
    ipcRenderer.invoke('vault:verifyPassphrase', passphrase),
  resetVault: (): Promise<void> => ipcRenderer.invoke('vault:resetVault'),
  listResters: (): Promise<string[]> => ipcRenderer.invoke('vault:listResters'),
  putRester: (passphrase: string, name: string, rester: VaultRester): Promise<void> =>
    ipcRenderer.invoke('vault:putRester', passphrase, name, rester),
  getRester: (passphrase: string, name: string): Promise<VaultRester> =>
    ipcRenderer.invoke('vault:getRester', passphrase, name),
  updateRester: (passphrase: string, name: string, rester: VaultRester): Promise<void> =>
    ipcRenderer.invoke('vault:updateRester', passphrase, name, rester),
  deleteRester: (name: string): Promise<void> => ipcRenderer.invoke('vault:deleteRester', name),
  testResterLogin: (
    api: VaultResterAPI,
    environment: VaultEnvironment,
    user: VaultResterUser
  ): Promise<VaultResterLoginTest> =>
    ipcRenderer.invoke('vault:testResterLogin', api, environment, user),
  listHTTPChannels: (): Promise<VaultHTTPChannel[]> => ipcRenderer.invoke('vault:listHTTPChannels'),
  createHTTPChannel: (
    passphrase: string,
    name: string,
    rester: string,
    environment: string,
    type: 'api' | 'web' | 'app',
    scope: VaultHTTPScope,
    webScope: VaultWebScope | null,
    ttlSeconds: number,
    askId?: string
  ): Promise<VaultHTTPChannelWithShortId> =>
    ipcRenderer.invoke(
      'vault:createHTTPChannel',
      passphrase,
      name,
      rester,
      environment,
      type,
      scope,
      webScope,
      ttlSeconds,
      askId
    ),
  activateHTTPChannel: (
    passphrase: string,
    id: string,
    ttlSeconds: number
  ): Promise<VaultHTTPChannelWithShortId> =>
    ipcRenderer.invoke('vault:activateHTTPChannel', passphrase, id, ttlSeconds),
  revokeHTTPChannel: (id: string): Promise<void> =>
    ipcRenderer.invoke('vault:revokeHTTPChannel', id),
  policyCatalog: (): Promise<VaultPolicyRule[]> => ipcRenderer.invoke('vault:policyCatalog'),
  listPolicies: (): Promise<VaultPolicies> => ipcRenderer.invoke('vault:listPolicies'),
  putPolicy: (passphrase: string, projectId: string | null, policy: VaultPolicy): Promise<void> =>
    ipcRenderer.invoke('vault:putPolicy', passphrase, projectId, policy),
  migratePolicy: (passphrase: string): Promise<void> =>
    ipcRenderer.invoke('vault:migratePolicy', passphrase)
}

const windowControls = {
  minimize: (): void => ipcRenderer.send('window:minimize'),
  toggleMaximize: (): void => ipcRenderer.send('window:toggle-maximize'),
  close: (): void => ipcRenderer.send('window:close'),
  isMaximized: (): Promise<boolean> => ipcRenderer.invoke('window:is-maximized'),
  onMaximizedChange: (callback: (maximized: boolean) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, maximized: boolean): void =>
      callback(maximized)
    ipcRenderer.on('window:maximized', listener)
    return () => ipcRenderer.removeListener('window:maximized', listener)
  },
  onClosing: (callback: () => void): (() => void) => {
    const listener = (): void => callback()
    ipcRenderer.on('window:closing', listener)
    return () => ipcRenderer.removeListener('window:closing', listener)
  }
}

const workgroup = {
  onSpawn: (callback: (request: WorkgroupSpawnRequest) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, request: WorkgroupSpawnRequest): void =>
      callback(request)
    ipcRenderer.on('workgroup:spawn', listener)
    return () => ipcRenderer.removeListener('workgroup:spawn', listener)
  },
  reportSpawn: (requestId: string, result: WorkgroupSpawnResult): void =>
    ipcRenderer.send('workgroup:spawn-result', { requestId, ...result }),
  pending: (): Promise<WorkgroupPending[]> => ipcRenderer.invoke('workgroup:pending'),
  decide: (
    requestId: string,
    approved: boolean,
    reason?: string,
    accounts?: string[]
  ): Promise<boolean> =>
    ipcRenderer.invoke('workgroup:decide', { requestId, approved, reason, accounts }),
  onPendingChange: (callback: (pending: WorkgroupPending[]) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, pending: WorkgroupPending[]): void =>
      callback(pending)
    ipcRenderer.on('workgroup:pending-changed', listener)
    return () => ipcRenderer.removeListener('workgroup:pending-changed', listener)
  },
  onOpenApprovals: (callback: () => void): (() => void) => {
    const listener = (): void => callback()
    ipcRenderer.on('workgroup:open-approvals', listener)
    return () => ipcRenderer.removeListener('workgroup:open-approvals', listener)
  },
  list: (projectPath: string): Promise<WorkgroupInfo[]> =>
    ipcRenderer.invoke('workgroups:list', projectPath),
  thread: (
    projectPath: string,
    threadId: string,
    before?: number,
    limit?: number
  ): Promise<WorkgroupThreadResult> =>
    ipcRenderer.invoke('workgroups:thread', { projectPath, threadId, before, limit }),
  disband: (projectPath: string, groupId: number): Promise<WorkgroupDisbandResult> =>
    ipcRenderer.invoke('workgroups:disband', { projectPath, groupId }),
  join: (request: WorkgroupJoinRequest): Promise<WorkgroupJoinResult> =>
    ipcRenderer.invoke('workgroups:join', request)
}

const accounts = {
  list: (): Promise<AccountInfo[]> => ipcRenderer.invoke('accounts:list'),
  add: (name: string): Promise<AccountAddResult> => ipcRenderer.invoke('accounts:add', name),
  remove: (name: string): Promise<AccountRemoveResult> =>
    ipcRenderer.invoke('accounts:remove', name)
}

const usage = {
  get: (): Promise<UsageReport | null> => ipcRenderer.invoke('usage:get'),
  statusLine: (): Promise<StatusLineChainState | null> => ipcRenderer.invoke('statusline:state'),
  setStatusLine: (enabled: boolean): Promise<StatusLineChainState | null> =>
    ipcRenderer.invoke('statusline:set', enabled),
  contexts: (): Promise<Record<string, ContextUsage> | null> => ipcRenderer.invoke('usage:context'),
  onContexts: (callback: (tabs: Record<string, ContextUsage>) => void): (() => void) => {
    const listener = (
      _event: Electron.IpcRendererEvent,
      tabs: Record<string, ContextUsage>
    ): void => callback(tabs)
    ipcRenderer.on('usage:context-changed', listener)
    return () => ipcRenderer.removeListener('usage:context-changed', listener)
  }
}

const update = {
  state: (): Promise<UpdateState> => ipcRenderer.invoke('update:state'),
  check: (): Promise<UpdateState> => ipcRenderer.invoke('update:check'),
  install: (): Promise<void> => ipcRenderer.invoke('update:install'),
  restart: (): Promise<void> => ipcRenderer.invoke('update:restart'),
  onState: (callback: (state: UpdateState) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, state: UpdateState): void =>
      callback(state)
    ipcRenderer.on('update:state-changed', listener)
    return () => ipcRenderer.removeListener('update:state-changed', listener)
  }
}

const deps = {
  state: (): Promise<DepsState> => ipcRenderer.invoke('deps:state'),
  check: (): Promise<DepsState> => ipcRenderer.invoke('deps:check'),
  install: (name: string): Promise<void> => ipcRenderer.invoke('deps:install', name),
  setupDone: (): Promise<void> => ipcRenderer.invoke('deps:setupDone'),
  onState: (callback: (state: DepsState) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, state: DepsState): void => callback(state)
    ipcRenderer.on('deps:state-changed', listener)
    return () => ipcRenderer.removeListener('deps:state-changed', listener)
  }
}

const credits = {
  list: (): Promise<CreditsResult> => ipcRenderer.invoke('credits:list'),
  openChromiumLicenses: (): Promise<string> => ipcRenderer.invoke('credits:openChromiumLicenses')
}

const ask = {
  count: (): Promise<AskCount> => ipcRenderer.invoke('ask:count'),
  list: (passphrase: string): Promise<Ask[]> => ipcRenderer.invoke('ask:list', passphrase),
  get: (passphrase: string, id: string): Promise<Ask> =>
    ipcRenderer.invoke('ask:get', passphrase, id),
  execute: (passphrase: string, id: string, timeoutSeconds: number): Promise<Ask> =>
    ipcRenderer.invoke('ask:execute', passphrase, id, timeoutSeconds),
  stop: (passphrase: string, id: string): Promise<void> =>
    ipcRenderer.invoke('ask:stop', passphrase, id),
  decline: (passphrase: string, id: string, note: string): Promise<void> =>
    ipcRenderer.invoke('ask:decline', passphrase, id, note),
  onCount: (callback: (count: AskCount) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, count: AskCount): void => callback(count)
    ipcRenderer.on('ask:count', listener)
    return () => ipcRenderer.removeListener('ask:count', listener)
  },
  onChanged: (callback: () => void): (() => void) => {
    const listener = (): void => callback()
    ipcRenderer.on('ask:changed', listener)
    return () => ipcRenderer.removeListener('ask:changed', listener)
  },
  onOpen: (callback: (id: string) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, id: string): void => callback(id)
    ipcRenderer.on('ask:open', listener)
    return () => ipcRenderer.removeListener('ask:open', listener)
  }
}

const web = {
  attach: (tabId: string, bounds: WebBounds): Promise<void> =>
    ipcRenderer.invoke('web:attach', tabId, bounds),
  setBounds: (tabId: string, bounds: WebBounds): Promise<void> =>
    ipcRenderer.invoke('web:set-bounds', tabId, bounds),
  detach: (tabId: string): Promise<void> => ipcRenderer.invoke('web:detach', tabId),
  getState: (tabId: string): Promise<WebState> => ipcRenderer.invoke('web:get-state', tabId),
  onState: (callback: (state: WebState) => void): (() => void) => {
    const listener = (_event: Electron.IpcRendererEvent, state: WebState): void => callback(state)
    ipcRenderer.on('web:state', listener)
    return () => ipcRenderer.removeListener('web:state', listener)
  }
}

const api = {
  ask,
  pty,
  accounts,
  usage,
  update,
  deps,
  credits,
  projects,
  workspaceLayouts,
  uiState,
  tabSessions,
  biblio,
  git,
  review,
  conflictAgent,
  device,
  resterClient,
  node,
  vault,
  web,
  windowControls,
  workgroup
}

if (process.contextIsolated) {
  try {
    contextBridge.exposeInMainWorld('electron', electronAPI)
    contextBridge.exposeInMainWorld('api', api)
  } catch (error) {
    console.error(error)
  }
} else {
  // @ts-ignore (define in dts)
  window.electron = electronAPI
  // @ts-ignore (define in dts)
  window.api = api
}
