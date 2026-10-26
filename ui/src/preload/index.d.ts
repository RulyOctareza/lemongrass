// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ElectronAPI } from '@electron-toolkit/preload'
import type {
  AccountAddResult,
  AccountInfo,
  AccountRemoveResult,
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
  BiblioTree,
  BiblioFileResult,
  ScratchpadImageResult,
  ImageFileResult,
  GitStatus,
  GitDiff,
  GitActionResult,
  GitBlame,
  GitBranches,
  GitCommitInfo,
  GitLogResult,
  GitLogSearchMode,
  GitResetPreview,
  GitIncomingCommit,
  GitRevFiles,
  GitRevision,
  GitStash,
  NodeIssued,
  NodeIssueRequest,
  NodeState,
  WorkspaceLayoutState,
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
  WorkgroupDisbandResult,
  WorkgroupJoinRequest,
  WorkgroupJoinResult,
  WorkgroupInfo,
  WorkgroupPending,
  WorkgroupSpawnRequest,
  WorkgroupSpawnResult,
  WorkgroupThreadResult
} from './types'

interface Api {
  workgroup: {
    onSpawn: (callback: (request: WorkgroupSpawnRequest) => void) => () => void
    reportSpawn: (requestId: string, result: WorkgroupSpawnResult) => void
    pending: () => Promise<WorkgroupPending[]>
    decide: (
      requestId: string,
      approved: boolean,
      reason?: string,
      accounts?: string[]
    ) => Promise<boolean>
    onPendingChange: (callback: (pending: WorkgroupPending[]) => void) => () => void
    onOpenApprovals: (callback: () => void) => () => void
    list: (projectPath: string) => Promise<WorkgroupInfo[]>
    thread: (
      projectPath: string,
      threadId: string,
      before?: number,
      limit?: number
    ) => Promise<WorkgroupThreadResult>
    disband: (projectPath: string, groupId: number) => Promise<WorkgroupDisbandResult>
    join: (request: WorkgroupJoinRequest) => Promise<WorkgroupJoinResult>
  }
  accounts: {
    list: () => Promise<AccountInfo[]>
    add: (name: string) => Promise<AccountAddResult>
    remove: (name: string) => Promise<AccountRemoveResult>
  }
  pty: {
    spawn: (opts: PtySpawnOptions) => Promise<{ id: string }>
    write: (id: string, data: string) => void
    resize: (id: string, cols: number, rows: number) => void
    kill: (id: string) => void
    gracefulClose: (id: string) => void
    onData: (callback: (payload: PtyDataPayload) => void) => () => void
    onExit: (callback: (payload: PtyExitPayload) => void) => () => void
  }
  projects: {
    list: () => Promise<Project[]>
    add: () => Promise<Project | null>
    setTrust: (id: string, trusted: boolean) => Promise<Project | null>
  }
  tabSessions: {
    list: (projectPath: string) => Promise<Record<string, string>>
    clear: (projectPath: string) => Promise<void>
    title: (projectPath: string, tabId: string, title: string) => Promise<void>
    forget: (projectPath: string, tabId: string) => Promise<void>
  }
  workspaceLayouts: {
    load: (projectId: string) => Promise<WorkspaceLayoutState | null>
    save: (projectId: string, layout: WorkspaceLayoutState | null) => void
  }
  biblio: {
    tree: (projectPath: string) => Promise<BiblioTree | null>
    read: (projectPath: string, relativePath: string) => Promise<string | null>
    readFile: (projectPath: string, relativePath: string) => Promise<BiblioFileResult | null>
    write: (projectPath: string, relativePath: string, content: string) => Promise<boolean>
    createScratchpad: (
      projectPath: string,
      title: string,
      content: string
    ) => Promise<string | null>
    createScratchpadFile: (
      projectPath: string,
      folderPath: string,
      title: string,
      content: string
    ) => Promise<string | null>
    saveScratchpadImage: (
      projectPath: string,
      notePath: string,
      bytes: Uint8Array,
      mime: string
    ) => Promise<ScratchpadImageResult>
    readImageFile: (filePath: string) => Promise<ImageFileResult>
    archiveScratchpad: (projectPath: string, relativePath: string) => Promise<boolean>
    onChanged: (callback: (projectPath: string) => void) => () => void
  }
  git: {
    status: (projectPath: string) => Promise<GitStatus>
    diff: (projectPath: string, path: string, origPath?: string) => Promise<GitDiff>
    commit: (projectPath: string, message: string, paths?: string[]) => Promise<GitActionResult>
    push: (projectPath: string) => Promise<GitActionResult>
    lastMessage: (projectPath: string) => Promise<string>
    branches: (projectPath: string) => Promise<GitBranches>
    switchBranch: (projectPath: string, name: string, remote: boolean) => Promise<GitActionResult>
    revFiles: (projectPath: string, rev: GitRevision) => Promise<GitRevFiles>
    revDiff: (
      projectPath: string,
      rev: GitRevision,
      path: string,
      origPath?: string
    ) => Promise<GitDiff>
    stashList: (projectPath: string) => Promise<GitStash[]>
    stashPush: (
      projectPath: string,
      message: string,
      includeUntracked: boolean
    ) => Promise<GitActionResult>
    stashAction: (
      projectPath: string,
      ref: string,
      action: 'apply' | 'pop' | 'drop'
    ) => Promise<GitActionResult>
    fetch: (projectPath: string) => Promise<GitActionResult>
    incoming: (projectPath: string, target: string) => Promise<GitIncomingCommit[]>
    integrate: (
      projectPath: string,
      target: string,
      mode: 'merge' | 'rebase'
    ) => Promise<GitActionResult>
    finish: (projectPath: string, action: 'continue' | 'abort') => Promise<GitActionResult>
    log: (
      projectPath: string,
      mode: GitLogSearchMode,
      text: string,
      skip: number,
      limit: number
    ) => Promise<GitLogResult>
    logToken: (projectPath: string) => Promise<string>
    commitInfo: (projectPath: string, hash: string) => Promise<GitCommitInfo | null>
    blame: (projectPath: string, hash: string, path: string) => Promise<GitBlame>
    resetPreview: (projectPath: string, hash: string) => Promise<GitResetPreview>
    resetSoft: (projectPath: string, hash: string) => Promise<GitActionResult>
    amendMessage: (projectPath: string, message: string) => Promise<GitActionResult>
  }
  device: {
    list: () => Promise<DeviceListResult>
    start: (request: DeviceStartRequest) => Promise<DeviceStartResult>
    stop: (id: string) => void
    control: (id: string, message: Uint8Array) => void
    onPacket: (callback: (packet: DevicePacket) => void) => () => void
    onEnd: (callback: (end: DeviceEnd) => void) => () => void
  }
  node: {
    state: () => Promise<NodeState>
    saveTunnel: (
      provider: 'ngrok' | 'local',
      authtoken: string | null,
      domain: string,
      localPort: number
    ) => Promise<NodeState>
    openTunnel: (hours: number) => Promise<NodeState>
    closeTunnel: () => Promise<NodeState>
    issue: (req: NodeIssueRequest) => Promise<NodeIssued>
    update: (req: NodeUpdateRequest) => Promise<NodeState>
    revoke: (id: string) => Promise<NodeState>
    remove: (id: string) => Promise<NodeState>
  }
  web: {
    attach: (tabId: string, bounds: WebBounds) => Promise<void>
    setBounds: (tabId: string, bounds: WebBounds) => Promise<void>
    detach: (tabId: string) => Promise<void>
    getState: (tabId: string) => Promise<WebState>
    onState: (callback: (state: WebState) => void) => () => void
  }
  vault: {
    list: () => Promise<VaultChannel[]>
    create: (
      passphrase: string,
      name: string,
      dbName: string,
      scope: VaultScope,
      ttlSeconds: number
    ) => Promise<VaultChannelWithShortId>
    activate: (
      passphrase: string,
      id: string,
      ttlSeconds: number
    ) => Promise<VaultChannelWithShortId>
    revoke: (id: string) => Promise<void>
    listConnections: () => Promise<string[]>
    putCredential: (passphrase: string, name: string, connectionString: string) => Promise<void>
    getConnection: (passphrase: string, name: string) => Promise<string>
    updateConnection: (passphrase: string, name: string, connectionString: string) => Promise<void>
    deleteConnection: (name: string) => Promise<void>
    testConnection: (connectionString: string) => Promise<void>
    testSavedConnection: (passphrase: string, name: string) => Promise<void>
    listTables: (passphrase: string, name: string) => Promise<string[]>
    connectionSSHHosts: (passphrase: string) => Promise<Record<string, string>>
    listSSHHosts: () => Promise<string[]>
    resolveSSHHost: (alias: string) => Promise<ResolvedSSHHost>
    hasPassphrase: () => Promise<boolean>
    setPassphrase: (passphrase: string) => Promise<void>
    verifyPassphrase: (passphrase: string) => Promise<void>
    resetVault: () => Promise<void>
    listResters: () => Promise<string[]>
    putRester: (passphrase: string, name: string, rester: VaultRester) => Promise<void>
    getRester: (passphrase: string, name: string) => Promise<VaultRester>
    updateRester: (passphrase: string, name: string, rester: VaultRester) => Promise<void>
    deleteRester: (name: string) => Promise<void>
    testResterLogin: (
      api: VaultResterAPI,
      environment: VaultEnvironment,
      user: VaultResterUser
    ) => Promise<VaultResterLoginTest>
    listHTTPChannels: () => Promise<VaultHTTPChannel[]>
    createHTTPChannel: (
      passphrase: string,
      name: string,
      rester: string,
      environment: string,
      type: 'api' | 'web',
      scope: VaultHTTPScope,
      webScope: VaultWebScope | null,
      ttlSeconds: number
    ) => Promise<VaultHTTPChannelWithShortId>
    activateHTTPChannel: (
      passphrase: string,
      id: string,
      ttlSeconds: number
    ) => Promise<VaultHTTPChannelWithShortId>
    revokeHTTPChannel: (id: string) => Promise<void>
    policyCatalog: () => Promise<VaultPolicyRule[]>
    listPolicies: () => Promise<VaultPolicies>
    putPolicy: (passphrase: string, projectId: string | null, policy: VaultPolicy) => Promise<void>
    migratePolicy: (passphrase: string) => Promise<void>
  }
  windowControls: {
    minimize: () => void
    toggleMaximize: () => void
    close: () => void
    isMaximized: () => Promise<boolean>
    onMaximizedChange: (callback: (maximized: boolean) => void) => () => void
    onClosing: (callback: () => void) => () => void
  }
}

declare global {
  interface Window {
    electron: ElectronAPI
    api: Api
  }
}
