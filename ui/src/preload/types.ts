// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

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

export interface UiState {
  lastProjectId?: string
  zoomFactor?: number
  startup?: 'lastProject' | 'projectSelector'
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

export type BiblioFileResult =
  | { kind: 'text'; text: string }
  | { kind: 'image'; modifiedMs: number }
  | { kind: 'binary' }
  | { kind: 'tooLarge' }

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
  rules: Record<string, 'allow' | 'deny' | 'ask'>
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
  mode: 'deny' | 'ask'
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
  // Only on a conflicted file: the conflict marker blocks left in its working file, or null when
  // that file is missing, binary or has a marker left open.
  conflictBlocks?: number | null
}

export interface GitOperationRefs {
  from: string
  onto: string
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
  // Set while operation is: a merge brings from into onto (the current branch), a rebase replays
  // from onto its upstream onto. The resolver's pane headers use the same names.
  operationRefs: GitOperationRefs | null
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

// ahead and behind count against upstream, and are 0 when there is none or it is gone.
export interface GitLocalBranch {
  name: string
  upstream: string | null
  ahead: number
  behind: number
}

export interface GitBranches {
  current: string | null
  local: GitLocalBranch[]
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

// diverged marks a push the remote refused because it has commits the branch does not.
export type GitPushResult = { ok: true } | { ok: false; error: string; diverged?: boolean }

// Diverged means both sides have commits the other lacks, so only a rebase brings them together.
export type GitUpdateResult =
  | { status: 'updated' }
  | { status: 'diverged'; branch: string; upstream: string; ahead: number; behind: number }
  // The remote deleted the branch, and its stale remote-tracking ref was dropped.
  | { status: 'gone'; message: string }
  | { status: 'error'; error: string }

// Where a forecast rebase stops: the commit's place among those replayed, both 1-based.
export interface GitForecastStop {
  index: number
  total: number
  subject: string
}

// What a merge or a rebase would do, worked out in memory before it runs. stop is null for a merge.
// needsDependency: the forecast cannot run because the named dependency is missing or too old.
export type GitForecast =
  | { status: 'clean' }
  | { status: 'conflict'; paths: string[]; stop: GitForecastStop | null }
  | { status: 'unavailable'; error: string }
  | { status: 'needsDependency'; dep: string }

// The switch to main made before deleting the checked-out branch. It stands even when the delete
// itself then fails.
export interface GitDeleteMove {
  from: string
  to: string
  carry: GitCarry
  notice?: string
}

export type GitDeleteResult =
  | { ok: true; moved: GitDeleteMove | null }
  | { ok: false; error: string; moved: GitDeleteMove | null }

// How uncommitted changes fared across a branch move: none to carry, carried, or a conflict that
// left the carry stash in place.
export type GitCarry = 'none' | 'carried' | 'conflict'

// notice explains a carry that needs the human's attention, such as a kept stash.
export type GitMoveResult =
  | { ok: true; from: string; to: string; carry: GitCarry; notice?: string }
  | { ok: false; error: string }

// A conflict with no merge or rebase stopped is 'stash': an --autostash reapply, a carried checkout
// or a stash apply that conflicted. It has no Continue or Abort, and git keeps the stash.
export type GitConflictOperation = 'merge' | 'rebase' | 'stash'

// One version of a conflicted file from git's index. missing is a side that deleted the file, or
// no base when both sides added it. A submodule counts as binary.
export type GitConflictStage =
  { state: 'text'; text: string } | { state: 'missing' } | { state: 'binary' }

// 1-based and inclusive, so an empty range has end = start - 1.
export interface GitLineRange {
  start: number
  end: number
}

// Where a conflict sits in the working file on disk, its markers included. base is set only when
// the file was written with diff3 or zdiff3 markers.
export interface GitConflictDisk extends GitLineRange {
  left: GitLineRange
  base: GitLineRange | null
  right: GitLineRange
}

export type GitConflictChunk =
  | { kind: 'clean'; lines: string[]; disk: GitLineRange | null }
  | {
      kind: 'conflict'
      left: string[]
      base: string[]
      right: string[]
      disk: GitConflictDisk | null
    }

export type GitConflictFile =
  | {
      status: 'ok'
      path: string
      operation: GitConflictOperation
      // Ref names for the pane headers. Sides follow the operation, not the stage number: a merge
      // has the current branch (:2:) left and the merged branch (:3:) right, a rebase has the
      // branch being replayed (:3:) left and the upstream (:2:) right, and a stash has the stashed
      // changes (:3:) left and the current branch (:2:) right.
      leftRef: string
      rightRef: string
      base: GitConflictStage
      left: GitConflictStage
      right: GitConflictStage
      // git merge-file's result on the three stages, or null when a side is missing or binary or
      // there is no base, which is a pick-a-side case.
      chunks: GitConflictChunk[] | null
      finalNewline: boolean
      // The working file as it is on disk, null when it is missing or binary.
      working: string | null
      // working parsed by its own markers, null when it has none. Every disk range is set.
      workingChunks: GitConflictChunk[] | null
    }
  | { status: 'notConflicted' }
  | { status: 'error'; error: string }

export type GitConflictChoice = 'left' | 'right' | 'delete'

export type ConflictPane = 'left' | 'middle' | 'right'

// startLine and endLine are 1-based and inclusive, on the working file as it is on disk.
export interface ConflictAnchor {
  kind: 'conflict'
  path: string
  startLine: number
  endLine: number
  pane: ConflictPane
  operation: GitConflictOperation
  leftRef: string
  rightRef: string
}

// Later annotators add their own anchor kinds here, each with its formatter in reviewCompose.ts.
export type AnnotationAnchor = ConflictAnchor

export interface Annotation {
  id: string
  anchor: AnnotationAnchor
  // The commented text itself, so the model finds the spot even when the lines moved.
  quote: string
  comment: string
}

// A block the human resolved in the resolver, in the lines of the file as saved before submit.
export interface ConflictReviewFile {
  path: string
  resolved: GitLineRange[]
}

// What a review is about as a whole. Main adds the conflicted files the renderer leaves out and
// reads the blocks still in conflict from each file on disk.
export interface ConflictReviewScope {
  kind: 'conflict'
  operation: GitConflictOperation
  leftRef: string
  rightRef: string
  files: ConflictReviewFile[]
}

// Later annotators add their own scope kinds here, each with its formatter in reviewCompose.ts.
export type ReviewScope = ConflictReviewScope

export interface ReviewSubmission {
  scope: ReviewScope
  annotations: Annotation[]
  message: string
}

export type ReviewSubmitResult = { ok: true } | { ok: false; reason: string }

// Text typed into a busy tab would land in the middle of its turn or answer its open prompt.
export interface TabBusy {
  busy: boolean
  reason: string | null
}

// The project's open conflict thread as `lgrass git conflict status --json` prints it. scope is the
// conflict scope key and assignee the tab the human handed the conflict to.
export interface ConflictThreadStatus {
  threadId: string
  scope: string
  assignee: string
}

// Where a conflict thread message sits: lines 1-based and inclusive in the working file as it was
// when posted, and quote those lines, so the spot is found again after edits.
export interface ConflictMessageAnchor {
  path: string
  startLine: number
  endLine: number
  quote: string
}

// One message the human posts into the conflict thread, a line comment when anchored.
export interface ConflictHumanMessage {
  body: string
  anchor: ConflictMessageAnchor | null
}

export type ConflictStartResult = { ok: true; threadId: string } | { ok: false; reason: string }

export type ConflictActionResult = { ok: true } | { ok: false; reason: string }

// Pushed to the renderer when the picked tab runs `lgrass git conflict submit`. files are relative
// to the repository's top level.
export interface ConflictSubmitted {
  projectPath: string
  tabId: string
  files: string[]
}

export interface DeviceInfo {
  serial: string
  // adb's own state: 'device' when usable, otherwise e.g. 'unauthorized' or 'offline'.
  state: string
  model: string
}

// dep names the dependency whose absence caused the failure, so the view can offer to install it.
export type DeviceListResult =
  { ok: true; devices: DeviceInfo[] } | { ok: false; error: string; dep?: string }

// The renderer picks the session id so it can listen for packets before the stream starts.
export interface DeviceStartRequest {
  id: string
  serial: string
}

export type DeviceStartResult =
  | { ok: true; name: string; codec: string; width: number; height: number }
  | { ok: false; error: string; dep?: string }

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

export interface ResterClientPair {
  name: string
  value: string
}

export type ResterClientBodyKind = 'none' | 'json' | 'form'

// One saved request in a project's Rester Client. It holds no credential: the chosen rester user
// supplies auth when it is sent.
export interface ResterClientRequest {
  id: string
  name: string
  rester: string
  environment: string
  user: string
  method: string
  url: string
  headers: ResterClientPair[]
  bodyKind: ResterClientBodyKind
  json: string
  form: ResterClientPair[]
}

export interface ResterClientCall {
  rester: string
  environment: string
  user: string
  method: string
  path: string
  body: string
  contentType: string
  headers: ResterClientPair[]
}

export interface ResterClientResult {
  status: number
  headers: Record<string, string[]>
  body: string
  binary: boolean
  size: number
  truncated: boolean
  durationMs: number
  url: string
}

export type ResterClientDownload =
  | { kind: 'saved'; path: string; bytes: number; status: number; durationMs: number }
  | { kind: 'cancelled' }
  | { kind: 'inline'; result: ResterClientResult }

// One send of a request, kept in its history: the parameters it went out with and what came back.
// A response body longer than the history cap is cut, with bodyCut set.
export interface ResterClientRun {
  id: string
  at: string
  request: Omit<ResterClientRequest, 'id' | 'name'>
  result: ResterClientResult | null
  download: ResterClientDownload | null
  error: string
  bodyCut: boolean
}

export type WorkspaceTab =
  | { id: string; kind: 'doc'; path: string }
  | { id: string; kind: 'diff'; path: string }
  | { id: string; kind: 'conflict'; path: string }
  | { id: string; kind: 'history' }
  | { id: string; kind: 'connector' }
  | { id: string; kind: 'web'; shellTabId: string }
  | { id: string; kind: 'workgroup'; groupId: number }
  | { id: string; kind: 'device' }
  | { id: string; kind: 'resterClient' }
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
  commands: string[]
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

export type ThreadKind = 'adhoc' | 'workgroup' | 'conflict'

export type ThreadMessageKind = 'message' | 'note' | 'submit'

export interface WorkgroupThreadMessage {
  id: number
  // Empty for a human message, whose label is "You".
  tabId: string
  label: string
  body: string
  createdAt: string
  author: 'tab' | 'human'
  kind: ThreadMessageKind
  anchor: ConflictMessageAnchor | null
  // The paths a submit message lists, empty for every other kind.
  files: string[]
}

export interface WorkgroupThread {
  id: string
  title: string
  groupId: number
  kind: ThreadKind
  // scope and assignee are empty unless kind is conflict.
  scope: string
  assignee: string
  closed: boolean
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

// resets_at and seen_at are Unix seconds. reset is true once resets_at has passed: the agent has not reported since, but
// the window is known to be empty. seen_at is when the agent last reported.
export interface UsageWindow {
  label: string
  used_percent: number
  resets_at: number
  reset: boolean
}

export interface AccountUsage {
  account: string
  windows: UsageWindow[]
  seen_at: number
}

export interface CodexUsage {
  windows: UsageWindow[]
  plan_type?: string
  seen_at: number
}

// commands holds each account's own statusline command that lemongrass runs, keyed by account name.
export interface StatusLineChainState {
  enabled: boolean
  commands: Record<string, string>
}

// One Claude tab's context window use as of its latest response, from its statusline payload.
export interface ContextUsage {
  used_tokens: number
  window_tokens: number
  seen_at: number
}

export interface UsageReport {
  claude: AccountUsage[]
  codex: CodexUsage | null
  errors?: string[]
}

// unsupported: this build cannot update itself (a source build, or not installed from the rpm), and
// message says why. installing covers the polkit password prompt, which rpm cannot be told apart from.
export type UpdatePhase =
  | 'unsupported'
  | 'idle'
  | 'checking'
  | 'downloading'
  | 'verifying'
  | 'installing'
  | 'installed'
  | 'failed'

export interface UpdateState {
  phase: UpdatePhase
  current: string
  // The newest published release, null until a check succeeds.
  latest: string | null
  available: boolean
  checkedAt: number | null
  // Downloaded fraction from 0 to 1 while downloading, otherwise null.
  progress: number | null
  message: string | null
}

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

export type CreditGroup = 'fonts' | 'app' | 'tools' | 'electron'

// One bundled third-party component, as written by ui/scripts/licenses.ts.
export interface CreditEntry {
  group: CreditGroup
  name: string
  version: string
  // An SPDX expression.
  license: string
  homepage?: string
  text: string
}

export interface CreditsResult {
  entries: CreditEntry[]
  // Set when licenses.json is absent, as under `npm run dev`, which never runs the generator.
  missing: boolean
}

// One OS dependency as `lgrassd deps check --json` reports it. belowFloor and unknownVersion only
// occur for a dependency with a floor; floor is "" when there is none.
export type DepStatus = 'ok' | 'missing' | 'belowFloor' | 'unknownVersion'

// unknown is reported on an OS family whose origin probe is not built yet.
export type DepOrigin =
  { kind: 'package'; package: string } | { kind: 'manual' } | { kind: 'none' } | { kind: 'unknown' }

export interface DepOffer {
  action: 'install' | 'upgrade'
  package: string
  route: 'helper' | 'winget' | 'brew'
}

export interface DepEntry {
  name: string
  required: boolean
  // camelCase feature ids, mapped to their own copy in the renderer.
  features: string[]
  floor: string
  path: string
  version: string
  origin: DepOrigin
  status: DepStatus
  offer: DepOffer | null
  // Set only when no route reaches a usable version.
  message: string
}

export interface DepsReport {
  os: { family: string; id: string; version: string }
  deps: DepEntry[]
}

export interface DepInstall {
  name: string
  action: 'install' | 'upgrade'
  package: string
  // The newest lines of the package manager's output.
  lines: string[]
}

export interface DepOutcome {
  ok: boolean
  text: string
}

// failed: the check could not run, and message says why. unsupported is set when this build cannot
// install anything, such as a source build. setupDue: the startup layer has not completed for this version.
export interface DepsState {
  phase: 'checking' | 'ready' | 'failed'
  report: DepsReport | null
  message: string | null
  installing: DepInstall | null
  outcomes: Record<string, DepOutcome>
  unsupported: string | null
  setupDue: boolean
}

// An ask is a model's request to the human, held in the vault daemon (biblio ask-ops seam). Every
// text field but the ids is model output: render it as text, never as HTML.
export type AskType = 'db' | 'rester' | 'command'
export type AskState = 'waiting' | 'running' | 'answered' | 'declined' | 'expired'

export interface AskVerdict {
  rule: string
  description?: string
  mode: 'ask' | 'deny'
}

export interface AskResult {
  exit_code: number
  output_tail: string
  timed_out: boolean
  stopped: boolean
  started_at: string
  // A zero time while the command still runs.
  ended_at?: string
}

export interface Ask {
  id: string
  type: AskType
  tab_id: string
  cwd: string
  created_at: string
  state: AskState
  blocking: boolean
  reason?: string
  tables?: string[]
  message?: string
  command?: string
  rule?: string
  verdict?: AskVerdict | null
  never_execute?: boolean
  never_execute_reason?: string
  short_id?: string
  note?: string
  result?: AskResult | null
}

// Numbers and an opaque id only, readable while the vault is locked.
export interface AskCount {
  waiting: number
  blocking: number
  newestId: string
}
