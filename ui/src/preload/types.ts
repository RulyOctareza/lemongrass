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
  // Set only under books/, for the toc viewer's written and updated dates.
  createdMs?: number
  modifiedMs?: number
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

// One ssh config host as OpenSSH resolves it (`ssh -G`), shown read-only in the Connections form.
export interface ResolvedSSHHost {
  hostname: string
  user: string
  port: string
  proxyJump: string
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

export interface VaultHeader {
  Name: string
  Value: string
}

export interface VaultLoginStep {
  Action: 'goto' | 'fill' | 'click' | 'wait'
  URL: string
  Selector: string
  Field: string
  TimeoutSeconds: number
}

export interface VaultEnvironment {
  Name: string
  BaseURL: string
  Origin: string
  Headers: VaultHeader[] | null
}

export interface VaultUserOverride {
  Fields: Record<string, string> | null
  FieldTypes: Record<string, string> | null
  Token: string
}

export interface VaultResterUser {
  Name: string
  Fields: Record<string, string> | null
  FieldTypes: Record<string, string> | null
  Token: string
  Tags: string[] | null
  Environments: string[] | null
  Overrides: Record<string, VaultUserOverride> | null
}

export interface VaultResterAPI {
  LoginEndpoint: string
  TokenPath: string
  TTLOrigin: string
  FixedTTLSeconds: number
  TokenPlacement: VaultTokenPlacement
  Headers: VaultHeader[] | null
}

export interface VaultResterWeb {
  Login: VaultLoginStep[] | null
}

export interface VaultAppStep {
  Action: 'launch' | 'tap' | 'fill' | 'key' | 'wait'
  Text: string
  ID: string
  Field: string
  Key: string
  TimeoutSeconds: number
}

export interface VaultResterApp {
  Package: string
  Login: VaultAppStep[] | null
}

export interface VaultRester {
  API: VaultResterAPI | null
  Web: VaultResterWeb | null
  App: VaultResterApp | null
  Environments: VaultEnvironment[]
  Users: VaultResterUser[]
}

export interface VaultResterLoginTest {
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

export interface VaultWebScope {
  Origins: string[] | null
  Exclusions: VaultMethodPath[] | null
}

export interface VaultHTTPChannel {
  ID: string
  Name: string
  Rester: string
  Environment: string
  Type: 'api' | 'web' | 'app'
  Scope: VaultHTTPScope
  WebScope: VaultWebScope | null
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

// Where a file sits in the change list, which also decides its color. Staged new is an added file
// in the index; unstaged new is an untracked one.
export type GitFileGroup =
  'conflicted' | 'changed' | 'renamed' | 'stagedNew' | 'unstagedNew' | 'deleted'

export interface GitFile {
  path: string
  origPath?: string
  kind: GitFileKind
  group: GitFileGroup
  // The index differs from HEAD for this file, or it is in conflict.
  staged: boolean
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
  // A merge or rebase that stopped and waits to be continued or aborted.
  operation: 'merge' | 'rebase' | null
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

export interface GitBranches {
  current: string | null
  local: string[]
  remote: string[]
}

export interface GitStash {
  ref: string
  message: string
  time: number
}

// A commit or a stash, named so the main process can resolve the revision range itself.
export type GitRevision = { kind: 'commit'; hash: string } | { kind: 'stash'; ref: string }

export interface GitRevFile {
  path: string
  origPath?: string
  kind: GitFileKind
}

export type GitRevFiles =
  { status: 'ok'; files: GitRevFile[] } | { status: 'error'; message: string }

export interface GitIncomingCommit {
  hash: string
  author: string
  time: number
  subject: string
}

export interface GitRef {
  name: string
  kind: 'head' | 'branch' | 'remote' | 'tag'
}

export interface GitCommit {
  hash: string
  parents: string[]
  author: string
  time: number
  subject: string
  refs: GitRef[]
  // True when no remote branch contains the commit.
  unpushed: boolean
}

export type GitLogSearchMode = 'message' | 'author' | 'hash' | 'file'

export type GitLogResult =
  | { status: 'ok'; commits: GitCommit[]; hasMore: boolean; head: string | null }
  | { status: 'error'; message: string }

export interface GitCommitInfo {
  hash: string
  parents: string[]
  author: string
  email: string
  time: number
  message: string
}

export interface GitBlameLine {
  number: number
  hash: string
  author: string
  time: number
  text: string
}

export type GitBlame =
  | { status: 'ok'; lines: GitBlameLine[] }
  | { status: 'binary' }
  | { status: 'too-large' }
  | { status: 'error'; message: string }

// What a soft reset would take off the branch: the commits, and how many of them a remote has.
export type GitResetPreview =
  { ok: true; count: number; pushed: number } | { ok: false; error: string }

export type GitActionResult = { ok: true } | { ok: false; error: string }

export interface DeviceInfo {
  serial: string
  // adb's own state: 'device' when usable, otherwise e.g. 'unauthorized' or 'offline'.
  state: string
  model: string
}

export type DeviceListResult = { ok: true; devices: DeviceInfo[] } | { ok: false; error: string }

// The renderer picks the session id so it can listen for packets before the stream starts.
export interface DeviceStartRequest {
  id: string
  serial: string
}

export type DeviceStartResult =
  | { ok: true; name: string; codec: string; width: number; height: number }
  | { ok: false; error: string }

// One H.264 unit from scrcpy, Annex B. A config packet holds SPS and PPS and must be
// decoded together with the frame that follows it.
export interface DevicePacket {
  id: string
  config: boolean
  key: boolean
  pts: number
  data: Uint8Array
}

export interface DeviceEnd {
  id: string
  error?: string
  // Another Device tab started mirroring the same phone, so this one must not reconnect.
  replaced?: boolean
}

export type WorkspaceTab =
  | { id: string; kind: 'doc'; path: string }
  | { id: string; kind: 'diff'; path: string }
  | { id: string; kind: 'history' }
  | { id: string; kind: 'connector' }
  | { id: string; kind: 'web'; shellTabId: string }
  | { id: string; kind: 'workgroup'; groupId: number }
  | { id: string; kind: 'device' }
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
  connecting?: boolean
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

// A rectangle in DIPs, as getBoundingClientRect reports it in the content area.
export interface WebBounds {
  x: number
  y: number
  width: number
  height: number
}

export interface WebState {
  tabId: string
  url: string
  title: string
  user: string
  open: boolean
}
