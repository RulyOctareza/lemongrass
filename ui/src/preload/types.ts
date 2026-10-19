// Shared preload/renderer type contracts. Kept separate from index.ts, which
// imports 'electron' and other main-process-only packages the renderer's own
// tsconfig can't resolve -- the renderer and index.d.ts both import from here
// instead of dragging that implementation file into the web program.

export interface PtySpawnOptions {
  command: string
  args?: string[]
  cwd?: string
  tabId?: string
  account?: string
  orientation?: 'tab' | 'thinker'
  cols?: number
  rows?: number
}

export interface PtyDataPayload {
  id: string
  data: string
}

export interface PtyExitPayload {
  id: string
  exitCode: number
  signal?: number
}

export interface Project {
  id: string
  name: string
  path: string
  trusted?: boolean
}

export interface BiblioNode {
  name: string
  path: string
  type: 'dir' | 'file'
  children?: BiblioNode[]
}

export type ScratchpadImageResult =
  { path: string } | { error: 'too-large' | 'unsupported' | 'failed' }

export type ImageFileResult =
  { bytes: Uint8Array; mime: string } | { error: 'too-large' | 'unsupported' | 'failed' }

export interface BiblioTree {
  toc: BiblioNode | null
  children: BiblioNode[]
}

export interface VaultScope {
  Tables: string[]
  Operations: string[]
}

export interface VaultChannel {
  ID: string
  Name: string
  DBName: string
  Scope: VaultScope
  CreatedAt: string
  ExpiresAt: string
}

export interface VaultChannelWithShortId {
  channel: VaultChannel
  shortId: string
}

export interface VaultTokenPlacement {
  Kind: 'header' | 'cookie' | 'query'
  Name: string
  Prefix: string
}

export type VaultLoginFieldType = 'number' | 'boolean'

export interface VaultDomainUser {
  Name: string
  Fields: Record<string, string>
  FieldTypes: Record<string, VaultLoginFieldType> | null
  Token: string
  Tags: string[]
}

export interface VaultHeader {
  Name: string
  Value: string
}

export interface VaultDomain {
  BaseURL: string
  LoginEndpoint: string
  TokenPath: string
  TTLOrigin: string
  FixedTTLSeconds: number
  TokenPlacement: VaultTokenPlacement
  Users: VaultDomainUser[]
  Headers: VaultHeader[] | null
}

export interface VaultDomainLoginTest {
  error: string
  status: number
  body: string
}

export interface VaultMethodPath {
  Method: string
  PathPattern: string
}

export interface VaultHTTPScope {
  Methods: string[]
  Exclusions: VaultMethodPath[]
}

export interface VaultHTTPChannel {
  ID: string
  Name: string
  Domain: string
  Scope: VaultHTTPScope
  CreatedAt: string
  ExpiresAt: string
}

export interface VaultHTTPChannelWithShortId {
  channel: VaultHTTPChannel
  shortId: string
}

// The global policy uses only binaries and paths. A project's may also exclude global entries.
export interface VaultPolicy {
  rules: Record<string, 'allow' | 'deny' | 'approval'>
  binaries: string[]
  paths: string[]
  exclude_binaries: string[]
  exclude_paths: string[]
}

// A project missing from projects has nothing stored: the defaults plus the global policy.
export interface VaultPolicies {
  global: VaultPolicy
  projects: Record<string, VaultPolicy>
}

export interface VaultPolicyRule {
  id: string
  category: string
  description: string
  mode: 'deny' | 'approval'
  locked: boolean
}

export type GitFileKind =
  | 'modified'
  | 'added'
  | 'deleted'
  | 'renamed'
  | 'copied'
  | 'typechange'
  | 'untracked'
  | 'conflicted'

export interface GitFile {
  path: string
  origPath?: string
  kind: GitFileKind
}

export interface GitStatus {
  isRepo: boolean
  branch: string | null
  detached: boolean
  upstream: string | null
  ahead: number
  behind: number
  hasCommits: boolean
  pushTarget: string | null
  files: GitFile[]
}

export interface GitDiffLine {
  number: number
  text: string
  type: 'context' | 'add' | 'del'
}

export interface GitDiffRow {
  left: GitDiffLine | null
  right: GitDiffLine | null
}

export interface GitDiffHunk {
  header: string
  rows: GitDiffRow[]
}

export type GitDiff =
  | { status: 'ok'; hunks: GitDiffHunk[]; added: number; removed: number }
  | { status: 'empty' }
  | { status: 'binary' }
  | { status: 'too-large' }
  | { status: 'error'; message: string }

export type GitActionResult = { ok: true } | { ok: false; error: string }

export type WorkspaceTab =
  | { id: string; kind: 'doc'; path: string }
  | { id: string; kind: 'diff'; path: string }
  | { id: string; kind: 'connector' }
  | { id: string; kind: 'workgroup'; groupId: number }
  | {
      id: string
      kind: 'shell'
      label: string
      command: string
      cwd?: string
      account?: string
      thinker?: boolean
    }

export type WorkspaceLayoutNode =
  | { type: 'leaf'; id: string; tabs: WorkspaceTab[]; activeTabId: string | null }
  | {
      type: 'split'
      id: string
      direction: 'row' | 'column'
      children: WorkspaceLayoutNode[]
      sizes: number[]
    }

export interface WorkspaceLayoutState {
  root: WorkspaceLayoutNode | null
  focusedPaneId: string | null
  groupColors?: Record<string, string>
}

export interface WorkgroupSpawnMember {
  tabId: string
  label: string
  vendor: string
  model?: string
  prompt: string
  account?: string
}

export interface WorkgroupSpawnRequest {
  requestId: string
  projectPath: string
  leaderTabId: string
  // False when the approval carried no account choices, so Claude thinkers take the leader's account.
  accountsChosen: boolean
  members: WorkgroupSpawnMember[]
}

export interface WorkgroupSpawnResult {
  ok: boolean
  error?: string
}

export interface WorkgroupMemberInfo {
  tabId: string
  role: 'leader' | 'thinker' | 'collaborator'
  label: string
  vendor: string
}

export interface WorkgroupInfo {
  id: number
  name: string
  threadId: string
  leaderTabId: string
  members: WorkgroupMemberInfo[]
}

export interface WorkgroupPendingMember {
  label: string
  vendor: string
  model?: string
  skills: string[]
  prompt: string
}

export interface WorkgroupPending {
  requestId: string
  projectPath: string
  groupName: string
  leaderTabId: string
  leaderLabel: string
  // Present when the proposal adds thinkers to a live group: who is already in it.
  existingMembers?: string[]
  members: WorkgroupPendingMember[]
  createdAt: number
}

export interface WorkgroupThreadMessage {
  id: number
  tabId: string
  label: string
  body: string
  createdAt: string
}

export interface WorkgroupThread {
  id: number
  title: string
  groupId: number
  messages: WorkgroupThreadMessage[]
  more: boolean
}

export interface WorkgroupThreadResult {
  ok: boolean
  thread?: WorkgroupThread
  error?: string
}

export interface WorkgroupJoinRequest {
  projectPath: string
  groupId: number
  tabId: string
  label: string
}

export interface WorkgroupJoinResult {
  ok: boolean
  error?: string
  // Set when the tab joined but the welcome line could not be typed into it.
  warning?: string
}

export interface WorkgroupDisbandResult {
  ok: boolean
  error?: string
}

export interface AccountInfo {
  name: string
  dir: string
}

export type AccountAddResult = { ok: true; account: AccountInfo } | { ok: false; error: string }

export type AccountRemoveResult = { ok: true } | { ok: false; error: string }

export interface NodeShare {
  alias: string
  books: string[]
  tasks: string[]
  // File names without .md.
  laws: string[]
  handovers: string[]
}

export interface NodeGrantView {
  id: string
  label: string
  shares: NodeShare[]
  expiresAt: string
  revoked: boolean
  createdAt: string
  lastUsed?: string
}

export interface NodeTask {
  slug: string
  archived: boolean
}

// A law or handover file, named without .md.
export interface NodeDoc {
  name: string
  archived: boolean
}

export interface NodeProjectContent {
  projectId: string
  name: string
  alias: string
  // Book identities, title and creation date, as grants store them.
  books: string[]
  tasks: NodeTask[]
  laws: NodeDoc[]
  handovers: NodeDoc[]
}

export interface NodeStatus {
  running: boolean
  url?: string
  until?: string
  error?: string
  daemonDown?: boolean
}

export interface NodeTunnelSettings {
  provider: 'ngrok' | 'local'
  authtokenSet: boolean
  domain: string
  localPort: number
}

export interface NodeState {
  tunnel: NodeTunnelSettings
  status: NodeStatus
  grants: NodeGrantView[]
  projects: NodeProjectContent[]
}

export interface NodeIssueRequest {
  label: string
}

export interface NodeIssued {
  id: string
  token: string
}

export interface NodeUpdateRequest {
  id: string
  label: string
  expiresAt: string
  shares: NodeShare[]
}
