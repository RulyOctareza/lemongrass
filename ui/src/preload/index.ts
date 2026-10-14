import { contextBridge, ipcRenderer } from 'electron'
import { electronAPI } from '@electron-toolkit/preload'
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
  WorkspaceLayoutState,
  WorkgroupDisbandResult,
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
    ipcRenderer.invoke('tabSessions:forget', { projectPath, tabId })
}

const projects = {
  list: (): Promise<Project[]> => ipcRenderer.invoke('projects:list'),
  add: (): Promise<Project | null> => ipcRenderer.invoke('projects:add')
}

const workspaceLayouts = {
  load: (projectId: string): Promise<WorkspaceLayoutState | null> =>
    ipcRenderer.invoke('workspaceLayouts:load', projectId),
  save: (projectId: string, layout: WorkspaceLayoutState | null): void =>
    ipcRenderer.send('workspaceLayouts:save', { projectId, layout })
}

const biblio = {
  tree: (projectPath: string): Promise<BiblioTree | null> =>
    ipcRenderer.invoke('biblio:tree', projectPath),
  read: (projectPath: string, relativePath: string): Promise<string | null> =>
    ipcRenderer.invoke('biblio:read', projectPath, relativePath),
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
  push: (projectPath: string): Promise<GitActionResult> =>
    ipcRenderer.invoke('git:push', projectPath)
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
    ttlSeconds: number
  ): Promise<VaultChannelWithShortId> =>
    ipcRenderer.invoke('vault:create', passphrase, name, dbName, scope, ttlSeconds),
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
  hasPassphrase: (): Promise<boolean> => ipcRenderer.invoke('vault:hasPassphrase'),
  setPassphrase: (passphrase: string): Promise<void> =>
    ipcRenderer.invoke('vault:setPassphrase', passphrase),
  verifyPassphrase: (passphrase: string): Promise<void> =>
    ipcRenderer.invoke('vault:verifyPassphrase', passphrase),
  resetVault: (): Promise<void> => ipcRenderer.invoke('vault:resetVault'),
  listDomains: (): Promise<string[]> => ipcRenderer.invoke('vault:listDomains'),
  putDomain: (passphrase: string, name: string, domain: VaultDomain): Promise<void> =>
    ipcRenderer.invoke('vault:putDomain', passphrase, name, domain),
  getDomain: (passphrase: string, name: string): Promise<VaultDomain> =>
    ipcRenderer.invoke('vault:getDomain', passphrase, name),
  updateDomain: (passphrase: string, name: string, domain: VaultDomain): Promise<void> =>
    ipcRenderer.invoke('vault:updateDomain', passphrase, name, domain),
  deleteDomain: (name: string): Promise<void> => ipcRenderer.invoke('vault:deleteDomain', name),
  testDomainLogin: (domain: VaultDomain, user: VaultDomainUser): Promise<VaultDomainLoginTest> =>
    ipcRenderer.invoke('vault:testDomainLogin', domain, user),
  listHTTPChannels: (): Promise<VaultHTTPChannel[]> => ipcRenderer.invoke('vault:listHTTPChannels'),
  createHTTPChannel: (
    passphrase: string,
    name: string,
    domainName: string,
    scope: VaultHTTPScope,
    ttlSeconds: number
  ): Promise<VaultHTTPChannelWithShortId> =>
    ipcRenderer.invoke('vault:createHTTPChannel', passphrase, name, domainName, scope, ttlSeconds),
  activateHTTPChannel: (
    passphrase: string,
    id: string,
    ttlSeconds: number
  ): Promise<VaultHTTPChannelWithShortId> =>
    ipcRenderer.invoke('vault:activateHTTPChannel', passphrase, id, ttlSeconds),
  revokeHTTPChannel: (id: string): Promise<void> =>
    ipcRenderer.invoke('vault:revokeHTTPChannel', id),
  policyCatalog: (): Promise<VaultPolicyRule[]> => ipcRenderer.invoke('vault:policyCatalog'),
  getPolicy: (passphrase: string): Promise<VaultPolicy> =>
    ipcRenderer.invoke('vault:getPolicy', passphrase),
  putPolicy: (passphrase: string, policy: VaultPolicy): Promise<void> =>
    ipcRenderer.invoke('vault:putPolicy', passphrase, policy),
  activatePolicy: (passphrase: string): Promise<void> =>
    ipcRenderer.invoke('vault:activatePolicy', passphrase)
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
  decide: (requestId: string, approved: boolean, reason?: string): Promise<boolean> =>
    ipcRenderer.invoke('workgroup:decide', { requestId, approved, reason }),
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
    ipcRenderer.invoke('workgroups:disband', { projectPath, groupId })
}

const accounts = {
  list: (): Promise<AccountInfo[]> => ipcRenderer.invoke('accounts:list'),
  add: (name: string): Promise<AccountAddResult> => ipcRenderer.invoke('accounts:add', name),
  remove: (name: string): Promise<AccountRemoveResult> =>
    ipcRenderer.invoke('accounts:remove', name)
}

const api = {
  pty,
  accounts,
  projects,
  workspaceLayouts,
  tabSessions,
  biblio,
  git,
  node,
  vault,
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
