// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

export type EngineChoice = 'mysql' | 'mariadb' | 'postgres'

// The vault reads this query parameter as a host alias from the human's own ssh config and dials
// the database through it (dbgate/sshdial.go), so the form keeps it apart from the other params.
const SSH_PARAM = 'lgrass_ssh'

export interface ConnectionFields {
  engine: EngineChoice
  host: string
  port: string
  user: string
  password: string
  database: string
  sshHost: string
  // The query string, with its leading "?", that the form has no field for.
  query: string
}

export function composeConnectionString(f: ConnectionFields): string {
  const user = f.user.trim()
  const auth = user
    ? `${encodeURIComponent(user)}${f.password ? ':' + encodeURIComponent(f.password) : ''}@`
    : ''
  const port = f.port.trim()
  const host = f.host.trim()
  const hostPort = port ? `${host}:${port}` : host
  const database = f.database.trim()
  const sshHost = f.sshHost.trim()
  const ssh = sshHost ? `${f.query ? '&' : '?'}${SSH_PARAM}=${encodeURIComponent(sshHost)}` : ''
  return `${f.engine}://${auth}${hostPort}${database ? '/' + database : ''}${f.query}${ssh}`
}

export function parseConnectionString(connectionString: string): ConnectionFields {
  const url = new URL(connectionString)
  const scheme = url.protocol.replace(/:$/, '').toLowerCase()
  const engine = scheme === 'postgresql' ? 'postgres' : scheme
  if (engine !== 'mysql' && engine !== 'mariadb' && engine !== 'postgres') {
    throw new Error(`Unsupported engine "${scheme}".`)
  }
  // The query is rewritten only when it carries the ssh alias, so any other string keeps its
  // params byte for byte.
  let query = url.search
  let sshHost = ''
  const params = new URLSearchParams(url.search)
  if (params.has(SSH_PARAM)) {
    sshHost = params.get(SSH_PARAM) ?? ''
    params.delete(SSH_PARAM)
    const rest = params.toString()
    query = rest ? '?' + rest : ''
  }
  return {
    engine,
    host: url.hostname,
    port: url.port,
    user: decodeURIComponent(url.username),
    password: decodeURIComponent(url.password),
    database: url.pathname.replace(/^\//, ''),
    sshHost,
    query
  }
}
