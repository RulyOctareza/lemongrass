import { ElectronAPI } from '@electron-toolkit/preload'
import type {
  AccountAddResult,
  AccountInfo,
  AccountRemoveResult,
  PtySpawnOptions,
  PtyDataPayload,
  PtyExitPayload,
  Project,
  BiblioTree,
  ScratchpadImageResult,
  ImageFileResult,
  GitStatus,
  GitDiff,
  GitActionResult,
  NodeIssued,
  NodeIssueRequest,
  NodeState,
  WorkspaceLayoutState,
  VaultScope,
  VaultChannel,
  VaultChannelWithShortId,
  VaultDomain,
  VaultDomainUser,
  VaultDomainLoginTest,
  VaultHTTPScope,
  VaultHTTPChannel,
  VaultHTTPChannelWithShortId,
  VaultPolicy,
  VaultPolicyRule,
  WorkgroupDisbandResult,
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
    revoke: (id: string) => Promise<NodeState>
    remove: (id: string) => Promise<NodeState>
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
    hasPassphrase: () => Promise<boolean>
    setPassphrase: (passphrase: string) => Promise<void>
    verifyPassphrase: (passphrase: string) => Promise<void>
    resetVault: () => Promise<void>
    listDomains: () => Promise<string[]>
    putDomain: (passphrase: string, name: string, domain: VaultDomain) => Promise<void>
    getDomain: (passphrase: string, name: string) => Promise<VaultDomain>
    updateDomain: (passphrase: string, name: string, domain: VaultDomain) => Promise<void>
    deleteDomain: (name: string) => Promise<void>
    testDomainLogin: (domain: VaultDomain, user: VaultDomainUser) => Promise<VaultDomainLoginTest>
    listHTTPChannels: () => Promise<VaultHTTPChannel[]>
    createHTTPChannel: (
      passphrase: string,
      name: string,
      domainName: string,
      scope: VaultHTTPScope,
      ttlSeconds: number
    ) => Promise<VaultHTTPChannelWithShortId>
    activateHTTPChannel: (
      passphrase: string,
      id: string,
      ttlSeconds: number
    ) => Promise<VaultHTTPChannelWithShortId>
    revokeHTTPChannel: (id: string) => Promise<void>
    policyCatalog: () => Promise<VaultPolicyRule[]>
    getPolicy: (passphrase: string) => Promise<VaultPolicy>
    putPolicy: (passphrase: string, policy: VaultPolicy) => Promise<void>
    activatePolicy: (passphrase: string) => Promise<void>
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
