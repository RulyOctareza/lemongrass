// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import type { RedactHints } from './types'

const REDACTED = '[redacted]'

// Names that carry a credential, matched by pattern because a rester's own scheme is never the only one a page
// uses: a CSRF header, a refresh token key or a client secret field is as private as the login token.
const SENSITIVE_NAME =
  /token|secret|passw|authorization|api[-_]?key|session|jwt|csrf|cookie|credential/i
const SENSITIVE_HEADER = /token|secret|passw|auth|api[-_]?key|session|jwt|csrf|cookie|credential/i
// In an address these also carry an OAuth or SSO handoff, where `code` and `state` are one-time credentials.
const SENSITIVE_PARAM = new RegExp(
  `${SENSITIVE_NAME.source}|^(code|state|sig|signature|key|otp)$`,
  'i'
)

function placedName(hints: RedactHints, kind: 'header' | 'cookie' | 'query'): string {
  return hints.tokenPlacement?.Kind === kind ? hints.tokenPlacement.Name.toLowerCase() : ''
}

export function redactHeaders(
  headers: Record<string, string>,
  hints: RedactHints
): Record<string, string> {
  const placed = placedName(hints, 'header')
  const out: Record<string, string> = {}
  for (const [name, value] of Object.entries(headers)) {
    const key = name.toLowerCase()
    out[name] = SENSITIVE_HEADER.test(key) || key === placed ? REDACTED : value
  }
  return out
}

// Replaces the value of every sensitive parameter in an address, in its query and in its fragment: an SSO
// landing puts its code or token in either, and a single page app puts its own query after a `#/route?`.
export function redactUrl(url: string, hints: RedactHints): string {
  const placed = placedName(hints, 'query')
  return url.replace(/([?&#])([A-Za-z0-9_.%[\]-]+)=([^&#\s"'<>]*)/g, (match, lead, key, value) => {
    const name = String(key).toLowerCase()
    if (value === '' || !(SENSITIVE_PARAM.test(name) || name === placed)) return match
    return `${lead}${key}=${REDACTED}`
  })
}

function redactTree(node: unknown): unknown {
  if (Array.isArray(node)) return node.map(redactTree)
  if (typeof node !== 'object' || node === null) return node
  const out: Record<string, unknown> = {}
  for (const [key, value] of Object.entries(node)) {
    const hide = value !== null && typeof value !== 'boolean' && SENSITIVE_NAME.test(key)
    out[key] = hide ? REDACTED : redactTree(value)
  }
  return out
}

// The dotted path the rester reads its login token from; matched on the path itself so a key that does not
// look sensitive, such as data.jwt or data.value, is still hidden.
function redactTokenPath(tree: unknown, path: string[]): void {
  let node = tree
  for (const key of path.slice(0, -1)) {
    if (typeof node !== 'object' || node === null) return
    node = (node as Record<string, unknown>)[key]
  }
  const last = path[path.length - 1]
  if (typeof node === 'object' && node !== null && last in node)
    (node as Record<string, unknown>)[last] = REDACTED
}

const FORM_BODY = /^[^=&\s]+=[^&]*(&[^=&\s]+=[^&]*)*$/

function redactForm(body: string, hints: RedactHints): string {
  return body
    .split('&')
    .map((pair) => {
      const at = pair.indexOf('=')
      const key = pair.slice(0, at).toLowerCase()
      return SENSITIVE_PARAM.test(key) || key === placedName(hints, 'query')
        ? `${pair.slice(0, at)}=${REDACTED}`
        : pair
    })
    .join('&')
}

// A request or response body: JSON by key pattern and the token path, a form by key pattern, anything else as
// free text.
export function redactBody(body: string, hints: RedactHints): string {
  let tree: unknown
  try {
    tree = JSON.parse(body)
  } catch {
    return FORM_BODY.test(body.trim()) ? redactForm(body.trim(), hints) : redactText(body, hints)
  }
  const scrubbed = redactTree(tree)
  if (hints.tokenPath) redactTokenPath(scrubbed, hints.tokenPath.split('.'))
  return JSON.stringify(scrubbed)
}

const BEARER = /\b(Bearer|Basic)\s+[A-Za-z0-9._~+/=-]{8,}/gi
const JWT = /\beyJ[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]{5,}\.[A-Za-z0-9_-]*/g
const ADDRESS = /https?:\/\/[^\s"'<>)\]]+/g
const JSON_PAIR = new RegExp(
  `("(?:[^"\\\\]|\\\\.)*(?:${SENSITIVE_NAME.source})(?:[^"\\\\]|\\\\.)*"\\s*:\\s*)("(?:[^"\\\\]|\\\\.)*"|[0-9]+)`,
  'gi'
)
const LOOSE_PAIR = new RegExp(
  `\\b([A-Za-z0-9_.-]*(?:${SENSITIVE_NAME.source})[A-Za-z0-9_.-]*)=([^\\s&"'<>),;]+)`,
  'gi'
)

// Free text, such as console output or a snapshot: token shapes, addresses, and name and value pairs whose name
// looks like a credential.
export function redactText(text: string, hints: RedactHints): string {
  return text
    .replace(ADDRESS, (url) => redactUrl(url, hints))
    .replace(BEARER, `$1 ${REDACTED}`)
    .replace(JWT, REDACTED)
    .replace(JSON_PAIR, (_m, lead) => `${lead}"${REDACTED}"`)
    .replace(LOOSE_PAIR, (_m, name) => `${name}=${REDACTED}`)
}

function variants(value: string): string[] {
  return [value, encodeURIComponent(value), JSON.stringify(value).slice(1, -1)]
}

const MIN_SECRET_LENGTH = 4

// The one pass every text that leaves the engine goes through: first the user's own login values, in the plain,
// URL encoded and JSON escaped forms, then the pattern based redaction. Call sites sanitize what they know the
// structure of, and this catches whatever they did not.
export function scrubText(text: string, secrets: Iterable<string>, hints: RedactHints): string {
  const values = [...secrets]
    .filter((value) => value.length >= MIN_SECRET_LENGTH)
    .flatMap(variants)
    .sort((a, b) => b.length - a.length)
  let out = text
  for (const value of new Set(values)) out = out.split(value).join(REDACTED)
  return redactText(out, hints)
}

const SECRET_FIELD = /pass|pin|otp|code|token|secret|key|session/i

// The login values worth scrubbing from output: those whose field key says secret. A user name or an email is
// identity, which the model already knows and needs to read, so it stays, whatever the field is called.
export function secretFieldValues(fields: Record<string, string>, identities: string[]): string[] {
  const known = new Set(identities.map((name) => name.toLowerCase()))
  return Object.entries(fields)
    .filter(([key, value]) => SECRET_FIELD.test(key) && !known.has(value.toLowerCase()))
    .map(([, value]) => value)
}
