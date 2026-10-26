// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain } from 'electron'
import { createHash, randomBytes, randomUUID } from 'crypto'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  readFileSync,
  readdirSync,
  renameSync,
  writeFileSync
} from 'fs'
import * as net from 'net'
import { homedir } from 'os'
import { join } from 'path'
import { nodeSocketPath } from './daemons'
import type {
  NodeDoc,
  NodeGrantView,
  NodeIssued,
  NodeIssueRequest,
  NodeProjectContent,
  NodeShare,
  NodeState,
  NodeStatus,
  NodeTask,
  NodeUpdateRequest
} from '../preload/types'

// Mirrors internal/node/config.go; the Go daemon only reads this file.
interface NodeConfig {
  projects: { alias: string; projectId: string }[]
  grants: {
    id: string
    label: string
    shares: NodeShare[]
    expiresAt: string
    revoked: boolean
    createdAt: string
    tokenHash: string
  }[]
  tunnel: {
    provider: 'ngrok' | 'local'
    authtoken?: string
    domain?: string
    localPort?: number
    until: string
  }
}

interface RegistryProject {
  id: string
  name: string
  path: string
}

const STORE_DIR = join(homedir(), '.lemongrass')
const CONFIG_FILE = join(STORE_DIR, 'node.json')
const ZERO_TIME = '0001-01-01T00:00:00Z'
const DEFAULT_DAYS = 30

// Mirrors internal/node/content.go: these index every law and every handover, so they are never shared.
const LAWS_INDEX = 'summary'
const HANDOVER_INDEX = 'whiteboard'

// The daemon omits empty laws and handovers, and grants written before they existed have none.
function normalShare(s: Partial<NodeShare> & { alias: string }): NodeShare {
  return {
    alias: s.alias,
    books: [...new Set((s.books ?? []).map(bookIdentity))],
    tasks: [...new Set(s.tasks ?? [])],
    laws: [...new Set(s.laws ?? [])],
    handovers: [...new Set(s.handovers ?? [])]
  }
}

function shareSize(s: NodeShare): number {
  return s.books.length + s.tasks.length + s.laws.length + s.handovers.length
}

function loadConfig(): NodeConfig {
  const empty: NodeConfig = {
    projects: [],
    grants: [],
    tunnel: { provider: 'ngrok', until: ZERO_TIME }
  }
  if (!existsSync(CONFIG_FILE)) return empty
  const raw = JSON.parse(readFileSync(CONFIG_FILE, 'utf-8')) as Partial<NodeConfig>
  return {
    projects: raw.projects ?? [],
    grants: (raw.grants ?? []).map((g) => ({ ...g, shares: (g.shares ?? []).map(normalShare) })),
    tunnel: { ...empty.tunnel, ...raw.tunnel }
  }
}

// Written to a temporary file and renamed, so the daemon never reads half a file.
function saveConfig(cfg: NodeConfig): void {
  mkdirSync(STORE_DIR, { recursive: true, mode: 0o700 })
  const tmp = CONFIG_FILE + '.tmp'
  writeFileSync(tmp, JSON.stringify(cfg, null, 2), { mode: 0o600 })
  chmodSync(tmp, 0o600)
  renameSync(tmp, CONFIG_FILE)
}

function loadRegistry(): RegistryProject[] {
  try {
    return JSON.parse(readFileSync(join(STORE_DIR, 'projects.json'), 'utf-8'))
  } catch {
    return []
  }
}

interface DaemonStatus {
  running: boolean
  connecting?: boolean
  url?: string
  until?: string
  error?: string
  lastUsed?: Record<string, string>
}

// One op per connection, answered once, the same shape as internal/nodesvc.
function callDaemon(op: 'reload' | 'status'): Promise<DaemonStatus> {
  return new Promise((resolve, reject) => {
    const socket = net.createConnection(nodeSocketPath())
    const chunks: Buffer[] = []
    socket.setTimeout(40_000, () =>
      socket.destroy(new Error('node: the node daemon did not answer'))
    )
    socket.on('connect', () => socket.end(JSON.stringify({ op }) + '\n'))
    socket.on('data', (chunk) => chunks.push(chunk))
    socket.on('error', reject)
    socket.on('close', () => {
      try {
        const resp = JSON.parse(Buffer.concat(chunks).toString('utf-8'))
        if (!resp.ok) reject(new Error(resp.error || `node: ${op} failed`))
        else resolve(resp.status as DaemonStatus)
      } catch {
        reject(new Error('node: the node daemon did not answer'))
      }
    })
  })
}

async function daemonStatus(op: 'reload' | 'status'): Promise<DaemonStatus | null> {
  try {
    return await callDaemon(op)
  } catch {
    return null
  }
}

function realDirs(dir: string): string[] {
  try {
    return readdirSync(dir, { withFileTypes: true })
      .filter((e) => e.isDirectory() && !e.isSymbolicLink())
      .map((e) => e.name)
      .sort()
  } catch {
    return []
  }
}

// Regular .md files only, named without .md, the way the daemon matches them.
function markdownNames(dir: string): string[] {
  try {
    return readdirSync(dir, { withFileTypes: true })
      .filter((e) => e.isFile() && e.name.endsWith('.md'))
      .map((e) => e.name.slice(0, -3))
      .sort()
  } catch {
    return []
  }
}

// Mirrors node.docFile: the folder first, then its archive.
function docs(dir: string, index: string): NodeDoc[] {
  const active = markdownNames(dir).filter((n) => n !== index)
  const archived = markdownNames(join(dir, 'archive')).filter(
    (n) => n !== index && !active.includes(n)
  )
  return [
    ...active.map((name) => ({ name, archived: false })),
    ...archived.map((name) => ({ name, archived: true }))
  ]
}

// Mirrors node.BookIdentity: the title before any bracketed date or tags, so a grant stored as
// `title[date]` and a folder named `title[date][tags]` both match the plain folder `title`.
function bookIdentity(name: string): string {
  const open = name.indexOf('[')
  return open < 0 ? name : name.slice(0, open)
}

function aliasFor(cfg: NodeConfig, project: RegistryProject): string {
  const existing = cfg.projects.find((p) => p.projectId === project.id)
  if (existing) return existing.alias
  const base = project.name.replace(/[^A-Za-z0-9_.-]+/g, '-') || 'project'
  let alias = base
  for (let n = 2; cfg.projects.some((p) => p.alias === alias); n++) alias = `${base}-${n}`
  return alias
}

function projectContent(cfg: NodeConfig): NodeProjectContent[] {
  return loadRegistry().map((p) => {
    const biblio = join(p.path, 'biblio')
    const books = realDirs(join(biblio, 'books'))
      .map(bookIdentity)
      .filter((id) => id !== '')
    const tasks: NodeTask[] = [
      ...realDirs(join(biblio, 'scratchpad'))
        .filter((slug) => slug !== 'archive')
        .map((slug) => ({ slug, archived: false })),
      ...realDirs(join(biblio, 'scratchpad', 'archive')).map((slug) => ({ slug, archived: true }))
    ]
    return {
      projectId: p.id,
      name: p.name,
      alias: aliasFor(cfg, p),
      books,
      tasks,
      laws: docs(join(biblio, 'laws'), LAWS_INDEX),
      handovers: docs(join(biblio, 'handover'), HANDOVER_INDEX)
    }
  })
}

function toStatus(st: DaemonStatus | null): NodeStatus {
  if (!st) return { running: false, daemonDown: true }
  return {
    running: st.running,
    connecting: !!st.connecting,
    url: st.url,
    until: st.until,
    error: st.error
  }
}

async function state(op: 'reload' | 'status' = 'status'): Promise<NodeState> {
  const cfg = loadConfig()
  const st = await daemonStatus(op)
  const grants: NodeGrantView[] = cfg.grants.map((g) => ({
    id: g.id,
    label: g.label,
    shares: g.shares,
    expiresAt: g.expiresAt,
    revoked: g.revoked,
    createdAt: g.createdAt,
    lastUsed: st?.lastUsed?.[g.id]
  }))
  return {
    tunnel: {
      provider: cfg.tunnel.provider,
      authtokenSet: !!cfg.tunnel.authtoken,
      domain: cfg.tunnel.domain ?? '',
      localPort: cfg.tunnel.localPort ?? 0
    },
    status: toStatus(st),
    grants,
    projects: projectContent(cfg)
  }
}

// A new token reads nothing until resources are added to it, which never changes the token.
function issue(req: NodeIssueRequest): NodeIssued {
  const cfg = loadConfig()
  const token = 'lgn_' + randomBytes(32).toString('base64url')
  const now = new Date()
  const id = randomUUID()
  cfg.grants.push({
    id,
    label: req.label.trim() || 'Unnamed',
    shares: [],
    expiresAt: new Date(now.getTime() + DEFAULT_DAYS * 86_400_000).toISOString(),
    revoked: false,
    createdAt: now.toISOString(),
    tokenHash: createHash('sha256').update(token).digest('hex')
  })
  saveConfig(cfg)
  return { id, token }
}

// Edits one live grant in place, so its token and MCP setup stay the same.
function updateGrant(req: NodeUpdateRequest): void {
  const cfg = loadConfig()
  const grant = cfg.grants.find((g) => g.id === req.id)
  if (!grant) throw new Error('node: no such grant')
  if (grant.revoked) throw new Error('node: a revoked grant cannot change')
  const expiresAt = new Date(req.expiresAt)
  if (Number.isNaN(expiresAt.getTime())) throw new Error('node: the expiry is not a date')
  // An empty list is valid: the person stays connected and reads nothing.
  const shares = req.shares.map(normalShare).filter((s) => shareSize(s) > 0)
  const registry = loadRegistry()
  for (const s of shares) {
    if (cfg.projects.some((p) => p.alias === s.alias)) continue
    const project = registry.find((p) => aliasFor(cfg, p) === s.alias)
    if (!project) throw new Error(`node: no project is named ${s.alias}`)
    cfg.projects.push({ alias: s.alias, projectId: project.id })
  }
  grant.label = req.label.trim() || 'Unnamed'
  grant.expiresAt = expiresAt.toISOString()
  grant.shares = shares
  saveConfig(cfg)
}

export function registerNodeHandlers(): void {
  ipcMain.handle('node:state', () => state())

  ipcMain.handle(
    'node:saveTunnel',
    (
      _e,
      provider: 'ngrok' | 'local',
      authtoken: string | null,
      domain: string,
      localPort: number
    ) => {
      const cfg = loadConfig()
      cfg.tunnel.provider = provider
      if (authtoken !== null) cfg.tunnel.authtoken = authtoken.trim() || undefined
      cfg.tunnel.domain = domain.trim() || undefined
      cfg.tunnel.localPort = localPort > 0 ? localPort : undefined
      saveConfig(cfg)
      return state('reload')
    }
  )

  ipcMain.handle('node:openTunnel', (_e, hours: number) => {
    const cfg = loadConfig()
    cfg.tunnel.until = new Date(Date.now() + Math.max(0.25, hours) * 3_600_000).toISOString()
    saveConfig(cfg)
    return state('reload')
  })

  ipcMain.handle('node:closeTunnel', () => {
    const cfg = loadConfig()
    cfg.tunnel.until = ZERO_TIME
    saveConfig(cfg)
    return state('reload')
  })

  ipcMain.handle('node:issue', async (_e, req: NodeIssueRequest) => {
    const issued = issue(req)
    await daemonStatus('reload')
    return issued
  })

  ipcMain.handle('node:update', async (_e, req: NodeUpdateRequest) => {
    updateGrant(req)
    return state('reload')
  })

  ipcMain.handle('node:revoke', (_e, id: string) => {
    const cfg = loadConfig()
    const grant = cfg.grants.find((g) => g.id === id)
    if (grant) grant.revoked = true
    saveConfig(cfg)
    return state('reload')
  })

  ipcMain.handle('node:remove', (_e, id: string) => {
    const cfg = loadConfig()
    cfg.grants = cfg.grants.filter((g) => g.id !== id || !g.revoked)
    saveConfig(cfg)
    return state('reload')
  })
}
