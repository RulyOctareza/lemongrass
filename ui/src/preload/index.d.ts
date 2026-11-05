// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ElectronAPI } from '@electron-toolkit/preload'
import type {
  AccountAddResult,
  Ask,
  AskCount,
  AskDeclineExtra,
  AskTermAttach,
  AskTermData,
  AskTermEnd,
  Notice,
  NoticeInput,
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
  ProjectDeleteResult,
  UiState,
  BiblioTree,
  BiblioFileResult,
  ProjectFileResult,
  ProjectFilesChange,
  ProjectListResult,
  ProjectOpResult,
  ProjectOpsResult,
  ProjectSearchResult,
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
  NodeIssueRequest,
  ResterClientCall,
  ResterClientDownload,
  ResterClientRequest,
  ResterClientResult,
  ResterClientRun,
  NodeState,
  WorkspaceLayoutState,
  VaultScope,
  VaultChannel,
  VaultChannelWithShortId,
  WebBounds,
  WebState,
  VaultRester,
  VaultResterSummary,
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
  ask: {
    count: () => Promise<AskCount>
    list: (passphrase: string) => Promise<Ask[]>
    get: (passphrase: string, id: string) => Promise<Ask>
    // rows and cols are the card terminal's size, so the command starts at it.
    execute: (
      passphrase: string,
      id: string,
      timeoutSeconds: number,
      rows?: number,
      cols?: number
    ) => Promise<Ask>
    stop: (passphrase: string, id: string) => Promise<void>
    decline: (
      passphrase: string,
      id: string,
      note: string,
      extra?: AskDeclineExtra
    ) => Promise<void>
    termAttach: (passphrase: string, id: string) => Promise<AskTermAttach>
    termInput: (attachId: number, data: string) => void
    termResize: (attachId: number, rows: number, cols: number) => void
    termDetach: (attachId: number) => void
    onTermData: (callback: (payload: AskTermData) => void) => () => void
    onTermEnd: (callback: (payload: AskTermEnd) => void) => () => void
    onCount: (callback: (count: AskCount) => void) => () => void
    onChanged: (callback: () => void) => () => void
    onOpen: (callback: (id: string) => void) => () => void
  }
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
  usage: {
    get: () => Promise<UsageReport | null>
    statusLine: () => Promise<StatusLineChainState | null>
    setStatusLine: (enabled: boolean) => Promise<StatusLineChainState | null>
    contexts: () => Promise<Record<string, ContextUsage> | null>
    onContexts: (callback: (tabs: Record<string, ContextUsage>) => void) => () => void
  }
  notifications: {
    list: () => Promise<Notice[]>
    add: (input: NoticeInput) => Promise<void>
    markRead: (ids?: string[]) => Promise<void>
    clear: (ids?: string[]) => Promise<void>
    onChanged: (callback: (items: Notice[]) => void) => () => void
  }
  update: {
    state: () => Promise<UpdateState>
    check: () => Promise<UpdateState>
    install: () => Promise<void>
    restart: () => Promise<void>
    onState: (callback: (state: UpdateState) => void) => () => void
  }
  deps: {
    state: () => Promise<DepsState>
    check: () => Promise<DepsState>
    install: (name: string) => Promise<void>
    setupDone: () => Promise<void>
    onState: (callback: (state: DepsState) => void) => () => void
  }
  credits: {
    list: () => Promise<CreditsResult>
    openChromiumLicenses: () => Promise<string>
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
    home: () => Promise<string>
    liveShells: () => Promise<Record<string, number>>
    delete: (id: string) => Promise<ProjectDeleteResult | null>
  }
  tabSessions: {
    list: (projectPath: string) => Promise<Record<string, string>>
    clear: (projectPath: string) => Promise<void>
    title: (projectPath: string, tabId: string, title: string) => Promise<void>
    forget: (projectPath: string, tabId: string) => Promise<void>
    states: (projectPath: string) => Promise<Record<string, TabBusy>>
  }
  workspaceLayouts: {
    load: (projectId: string) => Promise<WorkspaceLayoutState | null>
    save: (projectId: string, layout: WorkspaceLayoutState | null) => void
  }
  uiState: {
    load: () => Promise<UiState>
    save: (patch: UiState) => void
    zoomPresets: () => Promise<number[]>
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
  projectFiles: {
    list: (projectPath: string, dir: string) => Promise<ProjectListResult>
    unwatch: (projectPath: string, dir: string) => void
    search: (projectPath: string, query: string) => Promise<ProjectSearchResult>
    // The project root's prefix inside its repository, "sub/dir/", or "" at the root or outside git.
    gitPrefix: (projectPath: string) => Promise<string>
    readFile: (projectPath: string, path: string) => Promise<ProjectFileResult | null>
    createFile: (projectPath: string, dir: string, name: string) => Promise<ProjectOpResult>
    createFolder: (projectPath: string, dir: string, name: string) => Promise<ProjectOpResult>
    rename: (projectPath: string, path: string, newName: string) => Promise<ProjectOpResult>
    move: (projectPath: string, paths: string[], destDir: string) => Promise<ProjectOpsResult>
    copy: (projectPath: string, paths: string[], destDir: string) => Promise<ProjectOpsResult>
    duplicate: (projectPath: string, path: string) => Promise<ProjectOpResult>
    trash: (projectPath: string, paths: string[]) => Promise<ProjectOpsResult>
    reveal: (projectPath: string, path: string) => void
    onChanged: (callback: (change: ProjectFilesChange) => void) => () => void
  }
  git: {
    status: (projectPath: string) => Promise<GitStatus>
    diff: (projectPath: string, path: string, origPath?: string) => Promise<GitDiff>
    commit: (projectPath: string, message: string, paths?: string[]) => Promise<GitActionResult>
    push: (projectPath: string) => Promise<GitPushResult>
    lastMessage: (projectPath: string) => Promise<string>
    branches: (projectPath: string) => Promise<GitBranches>
    switchBranch: (projectPath: string, name: string, remote: boolean) => Promise<GitMoveResult>
    createBranch: (projectPath: string, name: string, base: string) => Promise<GitMoveResult>
    branchNameValid: (projectPath: string, name: string) => Promise<boolean>
    updateBranch: (projectPath: string, name: string, remote: boolean) => Promise<GitUpdateResult>
    rebaseOntoUpstream: (projectPath: string, name: string) => Promise<GitActionResult>
    rebaseForecast: (projectPath: string, name: string) => Promise<GitForecast>
    deleteBranch: (projectPath: string, name: string) => Promise<GitDeleteResult>
    commitsOnlyOn: (projectPath: string, name: string) => Promise<number | null>
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
    mergeCount: (projectPath: string, source: string) => Promise<number>
    mergeForecast: (projectPath: string, source: string) => Promise<GitForecast>
    merge: (projectPath: string, source: string) => Promise<GitActionResult>
    finish: (projectPath: string, action: 'continue' | 'abort') => Promise<GitActionResult>
    conflict: (projectPath: string, path: string) => Promise<GitConflictFile>
    conflictApply: (projectPath: string, path: string, content: string) => Promise<GitActionResult>
    conflictSave: (projectPath: string, path: string, content: string) => Promise<GitActionResult>
    conflictPick: (
      projectPath: string,
      path: string,
      choice: GitConflictChoice
    ) => Promise<GitActionResult>
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
  review: {
    submit: (
      projectPath: string,
      tabId: string,
      submission: ReviewSubmission
    ) => Promise<ReviewSubmitResult>
  }
  conflictAgent: {
    status: (projectPath: string) => Promise<ConflictThreadStatus | null>
    start: (
      projectPath: string,
      tabId: string,
      resolved: ConflictReviewFile[]
    ) => Promise<ConflictStartResult>
    post: (
      projectPath: string,
      messages: ConflictHumanMessage[],
      resolved: ConflictReviewFile[]
    ) => Promise<ConflictActionResult>
    close: (projectPath: string) => Promise<ConflictActionResult>
    onSubmitted: (callback: (submitted: ConflictSubmitted) => void) => () => void
  }
  device: {
    list: () => Promise<DeviceListResult>
    start: (request: DeviceStartRequest) => Promise<DeviceStartResult>
    stop: (id: string) => void
    control: (id: string, message: Uint8Array) => void
    onPacket: (callback: (packet: DevicePacket) => void) => () => void
    onEnd: (callback: (end: DeviceEnd) => void) => () => void
  }
  resterClient: {
    request: (passphrase: string, call: ResterClientCall) => Promise<ResterClientResult>
    download: (passphrase: string, call: ResterClientCall) => Promise<ResterClientDownload>
    flush: (rester: string, environment: string, user: string) => Promise<number>
    load: () => Promise<ResterClientRequest[]>
    save: (requests: ResterClientRequest[]) => Promise<void>
    history: (requestId: string) => Promise<ResterClientRun[]>
    recordRun: (requestId: string, run: ResterClientRun) => Promise<ResterClientRun[]>
    deleteRun: (requestId: string, runId: string) => Promise<ResterClientRun[]>
    clearHistory: (requestId: string) => Promise<void>
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
      ttlSeconds: number,
      askId?: string
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
    listResterSummaries: (passphrase: string) => Promise<VaultResterSummary[]>
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
      type: 'api' | 'web' | 'app',
      scope: VaultHTTPScope,
      webScope: VaultWebScope | null,
      ttlSeconds: number,
      askId?: string
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
    isFullScreen: () => Promise<boolean>
    // The header's height in device pixels, so main can center the macOS traffic lights on it.
    setHeaderHeight: (dip: number) => void
    onFullScreenChange: (callback: (fullScreen: boolean) => void) => () => void
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
