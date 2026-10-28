// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain } from 'electron'
import * as net from 'net'
import { homedir } from 'os'
import { join } from 'path'
import { listSSHHosts, resolveSSHHost } from './sshHosts'

function vaultSocketPath(): string {
  return join(homedir(), '.lemongrass', 'vault.sock')
}

function agentSocketPath(): string {
  return join(homedir(), '.lemongrass', 'agent.sock')
}

interface RPCResponse<T> {
  ok: boolean
  error?: string
  payload?: T
}

// One request per connection, mirroring the Go side (gatekeeper/ipc.go, agent/ipc.go): write one
// JSON line, then read until the peer closes -- both daemons answer exactly once per connection.
function connectOnce<T>(socketPath: string, op: string, payload?: unknown): Promise<T> {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(socketPath)
    const chunks: Buffer[] = []
    socket.on('connect', () => {
      socket.end(JSON.stringify({ op, payload }) + '\n')
    })
    socket.on('data', (chunk) => chunks.push(chunk))
    socket.on('error', reject)
    socket.on('close', () => {
      if (chunks.length === 0) {
        reject(new Error(`lgrass: no response from ${op}`))
        return
      }
      try {
        const resp = JSON.parse(Buffer.concat(chunks).toString('utf-8')) as RPCResponse<T>
        if (!resp.ok) {
          reject(new Error(resp.error || `lgrass: ${op} failed`))
          return
        }
        resolve(resp.payload as T)
      } catch (err) {
        reject(err)
      }
    })
  })
}

function isConnectionRace(err: unknown): boolean {
  const code = (err as NodeJS.ErrnoException)?.code
  return code === 'ENOENT' || code === 'ECONNREFUSED'
}

function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

// Retries only the connection-race window right after daemons.ts spawns the vault/agent
// daemon on this session's own launch -- the socket file may not exist for the first call
// or two yet. Any other failure, including an op-level error the daemon itself returned,
// surfaces immediately rather than retrying.
async function call<T>(
  socketPath: string,
  op: string,
  payload?: unknown,
  deadline = Date.now() + 2000
): Promise<T> {
  try {
    return await connectOnce<T>(socketPath, op, payload)
  } catch (err) {
    if (!isConnectionRace(err) || Date.now() >= deadline) throw err
    await sleep(100)
    return call<T>(socketPath, op, payload, deadline)
  }
}

// Exported because the web engine reads a channel's login fields through web_login_fields; the
// reply must never be forwarded to a model.
export function vaultCall<T>(op: string, payload?: unknown): Promise<T> {
  return call<T>(vaultSocketPath(), op, payload)
}

function agentCall<T>(op: string, payload?: unknown): Promise<T> {
  return call<T>(agentSocketPath(), op, payload)
}

export interface Scope {
  Tables: string[]
  Operations: string[]
}

export interface Channel {
  ID: string
  Name: string
  DBName: string
  Scope: Scope
  CreatedAt: string
  ExpiresAt: string
}

export interface ChannelWithShortId {
  channel: Channel
  shortId: string
}

export interface TokenPlacement {
  Kind: 'header' | 'cookie' | 'query'
  Name: string
  Prefix: string
}

export interface Header {
  Name: string
  Value: string
}

export interface LoginStep {
  Action: 'goto' | 'fill' | 'click' | 'wait'
  URL: string
  Selector: string
  Field: string
  TimeoutSeconds: number
}

export interface Environment {
  Name: string
  BaseURL: string
  Origin: string
  Headers: Header[] | null
}

export interface UserOverride {
  Fields: Record<string, string> | null
  FieldTypes: Record<string, string> | null
  Token: string
}

export interface ResterUser {
  Name: string
  Fields: Record<string, string> | null
  FieldTypes: Record<string, string> | null
  Token: string
  Tags: string[] | null
  Environments: string[] | null
  Overrides: Record<string, UserOverride> | null
}

export interface ResterAPI {
  LoginEndpoint: string
  TokenPath: string
  TTLOrigin: string
  FixedTTLSeconds: number
  TokenPlacement: TokenPlacement
  Headers: Header[] | null
}

export interface ResterWeb {
  Login: LoginStep[] | null
}

export interface AppStep {
  Action: 'launch' | 'tap' | 'fill' | 'key' | 'wait'
  Text: string
  ID: string
  Field: string
  Key: string
  TimeoutSeconds: number
}

export interface ResterApp {
  Package: string
  Login: AppStep[] | null
}

export interface Rester {
  API: ResterAPI | null
  Web: ResterWeb | null
  App: ResterApp | null
  Environments: Environment[]
  Users: ResterUser[]
}

// error is empty when the login passed; status and body are the login endpoint's non-2xx answer.
export interface ResterLoginTest {
  error: string
  status: number
  body: string
}

export interface MethodPath {
  Method: string
  PathPattern: string
}

export interface HTTPScope {
  Methods: string[]
  Exclusions: MethodPath[]
}

export interface WebScope {
  Origins: string[] | null
  Exclusions: MethodPath[] | null
}

export interface HTTPChannel {
  ID: string
  Name: string
  Rester: string
  Environment: string
  Type: 'api' | 'web' | 'app'
  Scope: HTTPScope
  WebScope: WebScope | null
  CreatedAt: string
  ExpiresAt: string
}

export interface HTTPChannelWithShortId {
  channel: HTTPChannel
  shortId: string
}

async function listChannels(): Promise<Channel[]> {
  const { channels } = await vaultCall<{ channels: Channel[] }>('list_channels')
  return channels ?? []
}

async function listConnections(): Promise<string[]> {
  const { names } = await vaultCall<{ names: string[] }>('list_connections')
  return names ?? []
}

// Writes a connection's credential (engine folded into the connection string itself, e.g.
// `mysql://user:pass@host:port/db`) under the master passphrase. Only the human-facing edit
// form reads it back (getConnection), and no op reachable from the agent returns it.
async function putCredential(
  passphrase: string,
  name: string,
  connectionString: string
): Promise<void> {
  await vaultCall<void>('put_credential', {
    root_secret: passphrase,
    db_name: name,
    value: Buffer.from(connectionString, 'utf-8').toString('base64')
  })
}

// Registers with the agent right after, so the human has a short id to relay to the model --
// the root secret stops here, it never transits the agent (book chapter: "authority lives in
// the vault, not the agent").
async function createChannel(
  passphrase: string,
  name: string,
  dbName: string,
  scope: Scope,
  ttlSeconds: number
): Promise<ChannelWithShortId> {
  const { channel } = await vaultCall<{ channel: Channel }>('create_channel', {
    root_secret: passphrase,
    name,
    db_name: dbName,
    scope,
    ttl_seconds: ttlSeconds
  })
  const { short_id: shortId } = await agentCall<{ short_id: string }>('register_channel', {
    real_id: channel.ID
  })
  return { channel, shortId }
}

async function activateChannel(
  passphrase: string,
  id: string,
  ttlSeconds: number
): Promise<ChannelWithShortId> {
  const { channel } = await vaultCall<{ channel: Channel }>('activate', {
    root_secret: passphrase,
    id,
    ttl_seconds: ttlSeconds
  })
  const { short_id: shortId } = await agentCall<{ short_id: string }>('register_channel', {
    real_id: channel.ID
  })
  return { channel, shortId }
}

// Revoke only writes through to the vault. The vault is authoritative on validity regardless
// of what the agent's own short-id mapping still thinks it knows (book chapter: "Authority
// lives in the vault, not the agent") -- a stale agent-side mapping to a revoked channel just
// fails at the next Query, so there's nothing this call needs from the agent to be correct.
async function revokeChannel(id: string): Promise<void> {
  await vaultCall<void>('revoke', { id })
}

// Reads a stored connection string back in full, password included, for the edit form.
async function getConnection(passphrase: string, name: string): Promise<string> {
  const { connection_string: connectionString } = await vaultCall<{ connection_string: string }>(
    'get_connection',
    { root_secret: passphrase, db_name: name }
  )
  return connectionString
}

// Replaces a stored connection string and re-wraps it for every channel already minted from it.
async function updateConnection(
  passphrase: string,
  name: string,
  connectionString: string
): Promise<void> {
  await vaultCall<void>('update_connection', {
    root_secret: passphrase,
    db_name: name,
    connection_string: connectionString
  })
}

async function deleteConnection(name: string): Promise<void> {
  await vaultCall<void>('delete_connection', { db_name: name })
}

// Tests a connection string that hasn't been saved yet -- no passphrase involved, since
// nothing here is encrypted or stored.
async function testConnection(connectionString: string): Promise<void> {
  await vaultCall<void>('test_connection', { connection_string: connectionString })
}

// Tests an already-saved connection, decrypting it under the master passphrase to actually
// open it.
async function testSavedConnection(passphrase: string, name: string): Promise<void> {
  await vaultCall<void>('test_connection_saved', { root_secret: passphrase, db_name: name })
}

// Lists the tables in an already-saved connection's database, for the Channels form's table
// picker -- decrypts under the master passphrase to actually connect, same as testSavedConnection.
async function listTables(passphrase: string, name: string): Promise<string[]> {
  const { tables } = await vaultCall<{ tables: string[] }>('list_tables', {
    root_secret: passphrase,
    db_name: name
  })
  return tables ?? []
}

async function hasPassphrase(): Promise<boolean> {
  const { has_passphrase: has } = await vaultCall<{ has_passphrase: boolean }>('has_passphrase')
  return has
}

// Records passphrase as the vault's passphrase for future VerifyPassphrase checks. Only
// meaningful the first time -- callers must check hasPassphrase first.
async function setPassphrase(passphrase: string): Promise<void> {
  await vaultCall<void>('set_passphrase', { root_secret: passphrase })
}

// Checks passphrase against the vault's own recorded canary, independent of any real
// connection's reachability -- a failure here can only mean the passphrase is wrong.
async function verifyPassphrase(passphrase: string): Promise<void> {
  await vaultCall<void>('verify_passphrase', { root_secret: passphrase })
}

// Permanently discards every stored credential, channel, and the passphrase canary itself.
// The only way back from a forgotten passphrase, since nothing encrypted under it is
// recoverable without it.
async function resetVault(): Promise<void> {
  await vaultCall<void>('reset_vault')
}

async function listResters(): Promise<string[]> {
  const { names } = await vaultCall<{ names: string[] }>('list_resters')
  return names ?? []
}

async function putRester(passphrase: string, name: string, rester: Rester): Promise<void> {
  await vaultCall<void>('put_rester', { root_secret: passphrase, name, rester })
}

// Reads a stored rester back in full, passwords and tokens included, for the edit form.
async function getRester(passphrase: string, name: string): Promise<Rester> {
  const { rester } = await vaultCall<{ rester: Rester }>('get_rester', {
    root_secret: passphrase,
    name
  })
  return rester
}

// Replaces a stored rester and re-wraps it for every HTTP channel already minted from it.
async function updateRester(passphrase: string, name: string, rester: Rester): Promise<void> {
  await vaultCall<void>('update_rester', { root_secret: passphrase, name, rester })
}

async function deleteRester(name: string): Promise<void> {
  await vaultCall<void>('delete_rester', { name })
}

// Tests a rester user's login (or, for a bring-your-own-token user, validates the pasted
// token) with values straight out of the in-progress form, with no root secret and nothing stored.
async function testResterLogin(
  api: ResterAPI,
  environment: Environment,
  user: ResterUser
): Promise<ResterLoginTest> {
  const result = await vaultCall<Partial<ResterLoginTest> | undefined>('test_rester_login', {
    api,
    environment,
    user
  })
  return { error: result?.error ?? '', status: result?.status ?? 0, body: result?.body ?? '' }
}

async function listHTTPChannels(): Promise<HTTPChannel[]> {
  const { channels } = await vaultCall<{ channels: HTTPChannel[] }>('list_http_channels')
  return channels ?? []
}

async function createHTTPChannel(
  passphrase: string,
  name: string,
  rester: string,
  environment: string,
  type: 'api' | 'web' | 'app',
  scope: HTTPScope,
  webScope: WebScope | null,
  ttlSeconds: number
): Promise<HTTPChannelWithShortId> {
  const { channel } = await vaultCall<{ channel: HTTPChannel }>('create_http_channel', {
    root_secret: passphrase,
    name,
    rester,
    environment,
    type,
    scope,
    web_scope: webScope,
    ttl_seconds: ttlSeconds
  })
  const { short_id: shortId } = await agentCall<{ short_id: string }>('register_channel', {
    real_id: channel.ID
  })
  return { channel, shortId }
}

async function activateHTTPChannel(
  passphrase: string,
  id: string,
  ttlSeconds: number
): Promise<HTTPChannelWithShortId> {
  const { channel } = await vaultCall<{ channel: HTTPChannel }>('activate_http', {
    root_secret: passphrase,
    id,
    ttl_seconds: ttlSeconds
  })
  const { short_id: shortId } = await agentCall<{ short_id: string }>('register_channel', {
    real_id: channel.ID
  })
  return { channel, shortId }
}

async function revokeHTTPChannel(id: string): Promise<void> {
  await vaultCall<void>('revoke_http', { id })
}

// The global policy uses only binaries and paths. A project's may also exclude global entries.
export interface Policy {
  rules: Record<string, 'allow' | 'deny' | 'approval'>
  binaries: string[]
  paths: string[]
  exclude_binaries: string[]
  exclude_paths: string[]
}

export interface PolicyRule {
  id: string
  category: string
  description: string
  mode: 'deny' | 'approval'
  locked: boolean
}

async function policyCatalog(): Promise<PolicyRule[]> {
  const { rules } = await vaultCall<{ rules: PolicyRule[] }>('policy_catalog')
  return rules ?? []
}

export interface Policies {
  global: Policy
  projects: Record<string, Policy>
}

function normalizePolicy(policy?: Partial<Policy>): Policy {
  return {
    rules: policy?.rules ?? {},
    binaries: policy?.binaries ?? [],
    paths: policy?.paths ?? [],
    exclude_binaries: policy?.exclude_binaries ?? [],
    exclude_paths: policy?.exclude_paths ?? []
  }
}

// The policy is not secret, so reading it needs no passphrase. A project missing from projects has nothing stored: the defaults plus the global policy.
async function listPolicies(): Promise<Policies> {
  const { global, projects } = await vaultCall<{
    global?: Partial<Policy>
    projects?: Record<string, Partial<Policy>>
  }>('list_policies')
  const out: Record<string, Policy> = {}
  for (const [id, policy] of Object.entries(projects ?? {})) out[id] = normalizePolicy(policy)
  return { global: normalizePolicy(global), projects: out }
}

// A null projectId saves the global policy. The vault validates it and checks the passphrase.
async function putPolicy(
  passphrase: string,
  projectId: string | null,
  policy: Policy
): Promise<void> {
  await vaultCall<void>('put_policy', {
    root_secret: passphrase,
    project_id: projectId ?? undefined,
    policy
  })
}

// Moves a policy sealed by an earlier version into the readable policy file. A no-op once done.
async function migratePolicy(passphrase: string): Promise<void> {
  await vaultCall<void>('migrate_policy', { root_secret: passphrase })
}

// Saved connections that go through an ssh host alias, keyed by connection name.
async function connectionSSHHosts(passphrase: string): Promise<Record<string, string>> {
  const { hosts } = await vaultCall<{ hosts: Record<string, string> | null }>(
    'connection_ssh_hosts',
    { root_secret: passphrase }
  )
  return hosts ?? {}
}

export function registerVaultHandlers(): void {
  ipcMain.handle('vault:list', () => listChannels())
  ipcMain.handle(
    'vault:create',
    (_event, passphrase: string, name: string, dbName: string, scope: Scope, ttlSeconds: number) =>
      createChannel(passphrase, name, dbName, scope, ttlSeconds)
  )
  ipcMain.handle('vault:activate', (_event, passphrase: string, id: string, ttlSeconds: number) =>
    activateChannel(passphrase, id, ttlSeconds)
  )
  ipcMain.handle('vault:revoke', (_event, id: string) => revokeChannel(id))
  ipcMain.handle('vault:listConnections', () => listConnections())
  ipcMain.handle(
    'vault:putCredential',
    (_event, passphrase: string, name: string, connectionString: string) =>
      putCredential(passphrase, name, connectionString)
  )
  ipcMain.handle('vault:getConnection', (_event, passphrase: string, name: string) =>
    getConnection(passphrase, name)
  )
  ipcMain.handle(
    'vault:updateConnection',
    (_event, passphrase: string, name: string, connectionString: string) =>
      updateConnection(passphrase, name, connectionString)
  )
  ipcMain.handle('vault:deleteConnection', (_event, name: string) => deleteConnection(name))
  ipcMain.handle('vault:testConnection', (_event, connectionString: string) =>
    testConnection(connectionString)
  )
  ipcMain.handle('vault:testSavedConnection', (_event, passphrase: string, name: string) =>
    testSavedConnection(passphrase, name)
  )
  ipcMain.handle('vault:listTables', (_event, passphrase: string, name: string) =>
    listTables(passphrase, name)
  )
  ipcMain.handle('vault:connectionSSHHosts', (_event, passphrase: string) =>
    connectionSSHHosts(passphrase)
  )
  ipcMain.handle('vault:listSSHHosts', () => listSSHHosts())
  ipcMain.handle('vault:resolveSSHHost', (_event, alias: string) => resolveSSHHost(alias))
  ipcMain.handle('vault:hasPassphrase', () => hasPassphrase())
  ipcMain.handle('vault:setPassphrase', (_event, passphrase: string) => setPassphrase(passphrase))
  ipcMain.handle('vault:verifyPassphrase', (_event, passphrase: string) =>
    verifyPassphrase(passphrase)
  )
  ipcMain.handle('vault:resetVault', () => resetVault())

  ipcMain.handle('vault:listResters', () => listResters())
  ipcMain.handle('vault:putRester', (_event, passphrase: string, name: string, rester: Rester) =>
    putRester(passphrase, name, rester)
  )
  ipcMain.handle('vault:getRester', (_event, passphrase: string, name: string) =>
    getRester(passphrase, name)
  )
  ipcMain.handle('vault:updateRester', (_event, passphrase: string, name: string, rester: Rester) =>
    updateRester(passphrase, name, rester)
  )
  ipcMain.handle('vault:deleteRester', (_event, name: string) => deleteRester(name))
  ipcMain.handle(
    'vault:testResterLogin',
    (_event, api: ResterAPI, environment: Environment, user: ResterUser) =>
      testResterLogin(api, environment, user)
  )
  ipcMain.handle('vault:listHTTPChannels', () => listHTTPChannels())
  ipcMain.handle(
    'vault:createHTTPChannel',
    (
      _event,
      passphrase: string,
      name: string,
      rester: string,
      environment: string,
      type: 'api' | 'web' | 'app',
      scope: HTTPScope,
      webScope: WebScope | null,
      ttlSeconds: number
    ) => createHTTPChannel(passphrase, name, rester, environment, type, scope, webScope, ttlSeconds)
  )
  ipcMain.handle(
    'vault:activateHTTPChannel',
    (_event, passphrase: string, id: string, ttlSeconds: number) =>
      activateHTTPChannel(passphrase, id, ttlSeconds)
  )
  ipcMain.handle('vault:revokeHTTPChannel', (_event, id: string) => revokeHTTPChannel(id))

  ipcMain.handle('vault:policyCatalog', () => policyCatalog())
  ipcMain.handle('vault:listPolicies', () => listPolicies())
  ipcMain.handle(
    'vault:putPolicy',
    (_event, passphrase: string, projectId: string | null, policy: Policy) =>
      putPolicy(passphrase, projectId, policy)
  )
  ipcMain.handle('vault:migratePolicy', (_event, passphrase: string) => migratePolicy(passphrase))
}
