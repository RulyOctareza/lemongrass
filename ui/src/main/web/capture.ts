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
  // Started after the human clicked or typed in the watch tab and before the model's next action.
  human: boolean
}

export interface FileChooser {
  backendNodeId: number
  // selectSingle or selectMultiple.
  mode: string
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
// One request printed alone can afford a larger body than a list could.
const SINGLE_BODY_CAP = 8000
const LIST_CAP = 60

function cap(text: string, limit: number): string {
  return text.length > limit
    ? text.slice(0, limit) + `... (${text.length - limit} more characters)`
    : text
}

export class Capture {
  action = 0
  private humanClicks = 0
  private humanKeys = 0
  private humanRequests = 0
  private humanActive = false
  // Mouse presses the engine itself sent and has not seen arrive yet; any other press is the human's.
  private ownClicks = 0
  private cancelledDownloads: string[] = []
  private chooserWaiter: ((chooser: FileChooser) => void) | undefined
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
    this.humanClicks = this.humanKeys = this.humanRequests = 0
    this.humanActive = false
  }

  // A model action begins: requests from here on are the model's again.
  startAction(): number {
    this.humanActive = false
    // Every press the last action sent has arrived by now, so a count left over would only hide a human click.
    this.ownClicks = 0
    return ++this.action
  }

  // Called just before the engine sends a mouse press, so its arrival is not taken for the human's.
  expectClick(): void {
    this.ownClicks++
  }

  // A mouse press reached the page. Presses the engine sent arrive here too, so those are subtracted.
  mouseDown(): void {
    if (this.ownClicks > 0) {
      this.ownClicks--
      return
    }
    this.humanClicks++
    this.humanActive = true
  }

  // Whether the human clicked or typed since the model's last action began.
  get humanInputSinceAction(): boolean {
    return this.humanActive
  }

  noteCancelledDownload(name: string, size: number): void {
    const known = size > 0 ? ` (${size} bytes)` : ''
    this.cancelledDownloads.push(`${name}${known}`)
  }

  // The downloads cancelled since this was last printed, or nothing.
  takeDownloadNotes(): string {
    const names = this.cancelledDownloads
    this.cancelledDownloads = []
    if (names.length === 0) return ''
    return `download cancelled: ${names.join(', ')} started without download; use download <ref> --out <file-or-directory>`
  }

  // Resolves with the next file chooser the page opens while interception is on.
  nextFileChooser(): Promise<FileChooser> {
    return new Promise((resolve) => {
      this.chooserWaiter = resolve
    })
  }

  forgetFileChooser(): void {
    this.chooserWaiter = undefined
  }

  // A key reached the page from a real keyboard. Keys the engine sends over CDP never arrive here.
  keyDown(): void {
    this.humanKeys++
    this.humanActive = true
  }

  // What the human did in the watch tab since this was last printed, or nothing when the human did nothing.
  takeHumanSummary(): string {
    const { humanClicks: clicks, humanKeys: keys, humanRequests: requests } = this
    this.humanClicks = this.humanKeys = this.humanRequests = 0
    if (clicks === 0 && keys === 0 && requests === 0) return ''
    const parts = [
      `${clicks} click${clicks === 1 ? '' : 's'}`,
      `${keys} key${keys === 1 ? '' : 's'}`,
      `${requests} request${requests === 1 ? '' : 's'}`
    ]
    const list = requests > 0 ? ` (network --since ${this.action} marks them human)` : ''
    return `human input since your last call: ${parts.join(', ')}${list}`
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
      case 'Page.fileChooserOpened':
        return this.fileChooserOpened(params)
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
      done: false,
      human: this.humanActive
    }
    if (entry.human && this.isShown(entry)) this.humanRequests++
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

  private fileChooserOpened(params: Record<string, unknown>): void {
    const waiter = this.chooserWaiter
    this.chooserWaiter = undefined
    waiter?.({ backendNodeId: Number(params.backendNodeId), mode: String(params.mode) })
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

  // Counts only: the model lists what it needs with network and console, so a result never carries bodies or polls.
  actionSummary(action: number): string {
    const caused = this.requests.filter((e) => e.action === action && this.isShown(e))
    const errors = this.messages.filter((m) => m.action === action && isError(m.level)).length
    const out: string[] = []
    if (caused.length === 0) out.push('requests: none')
    else {
      const failed = caused.filter((e) => this.isFailure(e)).length
      const nonGet = caused.filter((e) => e.method !== 'GET').length
      const repeats = caused.length - new Set(caused.map((e) => `${e.method} ${e.url}`)).size
      const parts = [
        failed > 0 ? `${failed} failed` : '',
        nonGet > 0 ? `${nonGet} non-GET` : '',
        repeats > 0 ? `${repeats} repeats` : ''
      ].filter((p) => p !== '')
      const detail = parts.length > 0 ? ` (${parts.join(', ')})` : ''
      out.push(`requests: ${caused.length} new${detail}, list them with network --since ${action}`)
    }
    if (errors > 0)
      out.push(
        `console: ${errors} error${errors === 1 ? '' : 's'}, list them with console --errors`
      )
    return out.join('\n')
  }

  // One line per request, no bodies. Identical method, address and status fold into one line under the newest
  // id with ×N; the status is part of the key so a failure never hides among successes.
  network(options: { failed: boolean; since?: number }): string {
    const list = this.requests.filter(
      (e) =>
        this.isShown(e) &&
        (!options.failed || this.isFailure(e)) &&
        (options.since === undefined || e.action >= options.since)
    )
    if (list.length === 0) return 'no requests'
    const groups = fold(list, (e) => `${e.human ? '(human) ' : ''}${this.line(e)}`)
    const shown = groups.slice(-LIST_CAP)
    const out: string[] = []
    if (shown.length < groups.length)
      out.push(`(${groups.length - shown.length} older lines not shown)`)
    for (const group of shown)
      out.push(`r${group.last.id} (${actions(group)}) ${group.key}${times(group)}`)
    out.push('one request in full: network r<id>')
    return out.join('\n')
  }

  // One request with its redacted headers and capped bodies, or undefined when the id is unknown.
  request(id: number): string | undefined {
    const entry = this.requests.find((e) => e.id === id)
    if (!entry) return undefined
    const hints = this.hints()
    const human = entry.human ? '(human) ' : ''
    const out = [`r${entry.id} (action ${entry.action}) ${human}${this.line(entry)}`]
    const headers = Object.entries(redactHeaders(entry.requestHeaders, hints))
    if (headers.length > 0) out.push('request headers:', ...headers.map(([k, v]) => `  ${k}: ${v}`))
    if (entry.requestBody)
      out.push(`request body: ${cap(redactBody(entry.requestBody, hints), SINGLE_BODY_CAP)}`)
    if (entry.mime) out.push(`response type: ${entry.mime}`)
    if (entry.responseBody)
      out.push(`response body: ${cap(redactBody(entry.responseBody, hints), SINGLE_BODY_CAP)}`)
    else if (entry.resourceType === 'Document') out.push('response body: not read for a page load')
    return out.join('\n')
  }

  consoleLog(options: { errors: boolean }): string {
    const list = this.messages.filter((m) => !options.errors || isError(m.level))
    if (list.length === 0) return options.errors ? 'no console errors' : 'no console messages'
    const groups = fold(list, (m) => `${m.level}\n${m.text}`)
    const shown = groups.slice(-LIST_CAP)
    const out = shown.map(
      (g) => `[${g.last.level}] (${actions(g)}) ${cap(g.last.text, 500)}${times(g)}`
    )
    if (shown.length < groups.length)
      out.unshift(`(${groups.length - shown.length} older lines not shown)`)
    return out.join('\n')
  }
}

interface Group<T> {
  key: string
  first: T
  last: T
  count: number
}

// Folds entries with the same key into one group, ordered by each group's newest entry.
function fold<T extends { id: number }>(list: T[], keyOf: (entry: T) => string): Group<T>[] {
  const groups = new Map<string, Group<T>>()
  for (const entry of list) {
    const key = keyOf(entry)
    const group = groups.get(key)
    if (group) {
      group.last = entry
      group.count++
    } else groups.set(key, { key, first: entry, last: entry, count: 1 })
  }
  return [...groups.values()].sort((a, b) => a.last.id - b.last.id)
}

function actions(group: Group<{ action: number }>): string {
  const [from, to] = [group.first.action, group.last.action]
  return from === to ? `action ${to}` : `actions ${from}-${to}`
}

function times(group: Group<unknown>): string {
  return group.count > 1 ? ` ×${group.count}` : ''
}

function isError(level: string): boolean {
  return level === 'error' || level === 'exception' || level === 'assert'
}
