import type { Cdp } from './cdp'
import { redactBody, redactHeaders, redactText, redactUrl } from './redact'
import type { RedactHints } from './types'

export interface RequestEntry {
  id: number
  // The action counter when the request started: `network --since` and the action summaries filter on it.
  action: number
  requestId: string
  method: string
  url: string
  resourceType: string
  requestHeaders: Record<string, string>
  requestBody?: string
  status?: number
  statusText?: string
  mime?: string
  location?: string
  responseBody?: string
  failed?: string
  done: boolean
}

export interface ConsoleEntry {
  id: number
  action: number
  level: string
  text: string
}

const SHOWN_TYPES = new Set(['Document', 'XHR', 'Fetch', 'Ping'])
// Streams stay open on purpose, so they never count as a request in flight.
const NOT_IN_FLIGHT = new Set(['WebSocket', 'EventSource'])
const TEXT_MIME = /json|text\/|xml|javascript|x-www-form-urlencoded/i
const BODY_CAP = 2000
const LIST_CAP = 60

function cap(text: string, limit: number): string {
  return text.length > limit
    ? text.slice(0, limit) + `... (${text.length - limit} more characters)`
    : text
}

export class Capture {
  action = 0
  private requests: RequestEntry[] = []
  private byRequestId = new Map<string, RequestEntry>()
  private inflight = new Map<string, number>()
  private messages: ConsoleEntry[] = []
  private pending = new Set<Promise<void>>()
  private seq = 0

  private readonly cdp: Cdp
  private readonly hints: () => RedactHints

  constructor(cdp: Cdp, hints: () => RedactHints) {
    this.cdp = cdp
    this.hints = hints
  }

  // How long each request still open has been in flight.
  inFlightAges(now = Date.now()): number[] {
    return [...this.inflight.values()].map((started) => now - started)
  }

  // Drops everything captured so far. Used after login, so the login page's own requests are never listed.
  clear(): void {
    this.requests = []
    this.byRequestId.clear()
    this.messages = []
  }

  handle(method: string, params: Record<string, unknown>): void {
    switch (method) {
      case 'Network.requestWillBeSent':
        return this.requestStarted(params)
      case 'Network.responseReceived':
        return this.responseReceived(params)
      case 'Network.loadingFinished':
        return this.loadingFinished(String(params.requestId))
      case 'Network.loadingFailed':
        return this.loadingFailed(params)
      case 'Runtime.consoleAPICalled':
        return this.consoleCalled(params)
      case 'Runtime.exceptionThrown':
        return this.exceptionThrown(params)
      case 'Log.entryAdded':
        return this.logEntry(params)
    }
  }

  private requestStarted(params: Record<string, unknown>): void {
    const requestId = String(params.requestId)
    const request = params.request as {
      url: string
      method: string
      headers?: Record<string, string>
      postData?: string
      hasPostData?: boolean
    }
    const resourceType = String(params.type ?? 'Other')
    if (!NOT_IN_FLIGHT.has(resourceType))
      this.inflight.set(requestId, this.inflight.get(requestId) ?? Date.now())
    const previous = this.byRequestId.get(requestId)
    if (previous && params.redirectResponse) {
      // A redirect reuses the request id: the earlier hop ends with the redirect's own status.
      const redirect = params.redirectResponse as {
        status: number
        headers?: Record<string, string>
      }
      previous.status = redirect.status
      previous.location = redirect.headers?.['location'] ?? redirect.headers?.['Location']
      previous.done = true
    }
    const entry: RequestEntry = {
      id: ++this.seq,
      action: this.action,
      requestId,
      method: request.method,
      url: request.url,
      resourceType,
      requestHeaders: request.headers ?? {},
      requestBody: request.postData,
      done: false
    }
    this.requests.push(entry)
    this.byRequestId.set(requestId, entry)
    if (request.hasPostData && request.postData === undefined) {
      this.track(
        this.cdp
          .send<{ postData: string }>('Network.getRequestPostData', { requestId })
          .then((r) => {
            entry.requestBody = r.postData
          })
          .catch(() => undefined)
      )
    }
  }

  private responseReceived(params: Record<string, unknown>): void {
    const entry = this.byRequestId.get(String(params.requestId))
    if (!entry) return
    const response = params.response as {
      status: number
      statusText?: string
      mimeType?: string
      headers?: Record<string, string>
    }
    entry.status = response.status
    entry.statusText = response.statusText
    entry.mime = response.mimeType
    entry.location = response.headers?.['location'] ?? response.headers?.['Location']
  }

  private loadingFinished(requestId: string): void {
    this.inflight.delete(requestId)
    const entry = this.byRequestId.get(requestId)
    if (!entry) return
    entry.done = true
    if (entry.resourceType === 'Document' || !TEXT_MIME.test(entry.mime ?? '')) return
    this.track(
      this.cdp
        .send<{ body: string; base64Encoded: boolean }>('Network.getResponseBody', { requestId })
        .then((r) => {
          if (!r.base64Encoded) entry.responseBody = r.body
        })
        .catch(() => undefined)
    )
  }

  private loadingFailed(params: Record<string, unknown>): void {
    const requestId = String(params.requestId)
    this.inflight.delete(requestId)
    const entry = this.byRequestId.get(requestId)
    if (!entry) return
    entry.done = true
    entry.failed = params.canceled ? 'canceled' : String(params.errorText ?? 'failed')
  }

  private track(work: Promise<void>): void {
    this.pending.add(work)
    void work.finally(() => this.pending.delete(work))
  }

  // Waits for the response bodies still being fetched, so a listing right after an action has them.
  async settled(): Promise<void> {
    while (this.pending.size > 0) await Promise.all([...this.pending])
  }

  private addMessage(level: string, text: string): void {
    this.messages.push({
      id: ++this.seq,
      action: this.action,
      level,
      text: redactText(text, this.hints())
    })
  }

  private consoleCalled(params: Record<string, unknown>): void {
    const args = (params.args as { value?: unknown; description?: string; type?: string }[]) ?? []
    const text = args
      .map((a) => (a.value !== undefined ? String(a.value) : (a.description ?? a.type ?? '')))
      .join(' ')
    this.addMessage(String(params.type), text)
  }

  private exceptionThrown(params: Record<string, unknown>): void {
    const details = params.exceptionDetails as {
      text?: string
      url?: string
      lineNumber?: number
      exception?: { description?: string }
    }
    const where = details.url ? ` (${details.url}:${(details.lineNumber ?? 0) + 1})` : ''
    this.addMessage(
      'exception',
      (details.exception?.description ?? details.text ?? 'exception') + where
    )
  }

  private logEntry(params: Record<string, unknown>): void {
    const entry = params.entry as { level: string; text: string; url?: string }
    this.addMessage(
      entry.level === 'warning' ? 'warning' : entry.level,
      entry.text + (entry.url ? ` (${entry.url})` : '')
    )
  }

  private isShown(entry: RequestEntry): boolean {
    return SHOWN_TYPES.has(entry.resourceType) || entry.failed !== undefined
  }

  private isFailure(entry: RequestEntry): boolean {
    return entry.failed !== undefined || (entry.status !== undefined && entry.status >= 400)
  }

  private statusText(entry: RequestEntry): string {
    if (entry.failed) return `failed (${entry.failed})`
    if (entry.status === undefined) return entry.done ? 'no response' : 'pending'
    return String(entry.status)
  }

  private line(entry: RequestEntry): string {
    const arrow = entry.location ? ` -> ${redactUrl(entry.location, this.hints())}` : ''
    return `${entry.method} ${redactUrl(entry.url, this.hints())} ${this.statusText(entry)}${arrow}`
  }

  // The requests an action caused, short: bodies only for the failures, since those are what the model reads.
  actionSummary(action: number): string {
    const caused = this.requests.filter((e) => e.action === action && this.isShown(e))
    if (caused.length === 0) return 'requests: none'
    const out = ['requests:']
    for (const entry of caused.slice(0, LIST_CAP)) {
      out.push(`  ${this.line(entry)}`)
      if (this.isFailure(entry) && entry.responseBody)
        out.push(`    response: ${cap(redactBody(entry.responseBody, this.hints()), 500)}`)
    }
    if (caused.length > LIST_CAP) out.push(`  ... ${caused.length - LIST_CAP} more`)
    return out.join('\n')
  }

  // The consoles messages an action caused, or nothing when it logged none.
  actionConsole(action: number): string {
    const errors = this.messages.filter((m) => m.action === action && isError(m.level))
    if (errors.length === 0) return ''
    return ['console errors:', ...errors.slice(0, 10).map((m) => `  ${cap(m.text, 300)}`)].join(
      '\n'
    )
  }

  network(options: { failed: boolean; since?: number }): string {
    const hints = this.hints()
    const list = this.requests.filter(
      (e) =>
        this.isShown(e) &&
        (!options.failed || this.isFailure(e)) &&
        (options.since === undefined || e.action >= options.since)
    )
    if (list.length === 0) return 'no requests'
    const shown = list.slice(-LIST_CAP)
    const out: string[] = []
    if (shown.length < list.length)
      out.push(`(${list.length - shown.length} older requests not shown)`)
    for (const entry of shown) {
      out.push(`#${entry.id} (action ${entry.action}) ${this.line(entry)}`)
      const headers = Object.entries(redactHeaders(entry.requestHeaders, hints))
      if (headers.length > 0)
        out.push('  request headers: ' + headers.map(([k, v]) => `${k}: ${v}`).join('; '))
      if (entry.requestBody)
        out.push(`  request body: ${cap(redactBody(entry.requestBody, hints), BODY_CAP)}`)
      if (entry.responseBody)
        out.push(`  response body: ${cap(redactBody(entry.responseBody, hints), BODY_CAP)}`)
    }
    return out.join('\n')
  }

  consoleLog(options: { errors: boolean }): string {
    const list = this.messages.filter((m) => !options.errors || isError(m.level))
    if (list.length === 0) return options.errors ? 'no console errors' : 'no console messages'
    const shown = list.slice(-LIST_CAP)
    const out = shown.map((m) => `[${m.level}] (action ${m.action}) ${cap(m.text, 500)}`)
    if (shown.length < list.length)
      out.unshift(`(${list.length - shown.length} older messages not shown)`)
    return out.join('\n')
  }
}

function isError(level: string): boolean {
  return level === 'error' || level === 'exception' || level === 'assert'
}
