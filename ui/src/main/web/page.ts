import { WebContentsView, session, type WebContents } from 'electron'
import { Capture } from './capture'
import type { Cdp } from './cdp'
import { findClickable } from './clickable'
import { deriveNames } from './names'
import { formatSnapshot, type AXNode, type SnapshotRef } from './snapshot'
import { WebError, type RedactHints } from './types'

const LOAD_TIMEOUT_MS = 20_000
const SCREENSHOT_TIMEOUT_MS = 10_000

// Reports the document's last DOM change time, so the settle wait can tell when the page went quiet. It runs
// in every new document before the page's own scripts, under a name the page has no reason to touch.
const TICK_SCRIPT = `(() => {
  let last = performance.now()
  Object.defineProperty(window, '__lgrassTick', { get: () => last, configurable: false })
  new MutationObserver(() => { last = performance.now() })
    .observe(document, { subtree: true, childList: true, attributes: true, characterData: true })
})()`

const SECRET_INPUT = /passw|token|secret|otp|one-time-code/i

// A password input, or one whose name, id or autocomplete says it holds a credential, such as a one time code.
function isSecretInput(attrs: string[]): boolean {
  for (let i = 0; i + 1 < attrs.length; i += 2) {
    const [name, value] = [attrs[i], attrs[i + 1]]
    if (name === 'type' && value.toLowerCase() === 'password') return true
    if (['name', 'id', 'autocomplete'].includes(name) && SECRET_INPUT.test(value)) return true
  }
  return false
}

export function sleep(ms: number): Promise<void> {
  return new Promise((resolve) => setTimeout(resolve, ms))
}

export function isHttpUrl(url: string): boolean {
  try {
    const protocol = new URL(url).protocol
    return protocol === 'http:' || protocol === 'https:'
  } catch {
    return false
  }
}

// One browser page in its own in-memory session, driven over CDP. Everything that reaches the model is built
// by the callers from what this class returns, after redaction.
export class Page {
  readonly view: WebContentsView
  readonly capture: Capture
  readonly cdp: Cdp
  private readonly wc: WebContents
  private refs = new Map<string, SnapshotRef>()
  private firstLiveRef = 1
  private nextRef = 1
  private listeners = new Set<() => void>()

  constructor(partition: string, hints: () => RedactHints) {
    // No "persist:" prefix: the partition lives in memory and ends with the page.
    this.view = new WebContentsView({
      webPreferences: {
        partition,
        sandbox: true,
        contextIsolation: true,
        nodeIntegration: false,
        backgroundThrottling: false
      }
    })
    this.wc = this.view.webContents
    session
      .fromPartition(partition)
      .setPermissionRequestHandler((_wc, _permission, done) => done(false))
    this.wc.setWindowOpenHandler(({ url }) => {
      // A popup would be a page the agent cannot see, so it opens in this page instead.
      if (isHttpUrl(url)) void this.wc.loadURL(url).catch(() => undefined)
      return { action: 'deny' }
    })
    const guard = (event: Electron.Event, url: string): void => {
      if (!isHttpUrl(url)) event.preventDefault()
    }
    this.wc.on('will-navigate', (event) => guard(event, event.url))
    this.wc.on('will-redirect', (event) => guard(event, event.url))
    this.wc.on('did-navigate', () => this.emitChange())
    this.wc.on('did-navigate-in-page', () => this.emitChange())
    this.wc.on('page-title-updated', () => this.emitChange())

    const dbg = this.wc.debugger
    dbg.attach('1.3')
    this.cdp = {
      send: <T>(method: string, params?: object): Promise<T> =>
        dbg.sendCommand(method, params) as Promise<T>
    }
    this.capture = new Capture(this.cdp, hints)
    dbg.on('message', (_event, method, params) => {
      this.capture.handle(method, params as Record<string, unknown>)
      if (method === 'Page.javascriptDialogOpening') void this.answerDialog(params)
    })
    void this.enable()
  }

  private ready: Promise<void> | undefined

  private enable(): Promise<void> {
    this.ready ??= (async () => {
      // A view with no document has no renderer yet, and CDP commands to it never answer.
      await this.wc.loadURL('about:blank')
      for (const domain of ['Page', 'Network', 'Runtime', 'Log', 'DOM', 'Accessibility'])
        await this.cdp.send(`${domain}.enable`)
      // Without this a view in a hidden window drops mouse input, so the holder window can stay hidden.
      await this.cdp.send('Emulation.setFocusEmulationEnabled', { enabled: true })
      await this.cdp.send('Page.addScriptToEvaluateOnNewDocument', { source: TICK_SCRIPT })
    })()
    return this.ready
  }

  // Every command starts here, so nothing runs before the domains are enabled.
  async open(): Promise<void> {
    await this.enable()
  }

  onChange(listener: () => void): void {
    this.listeners.add(listener)
  }

  private emitChange(): void {
    for (const listener of this.listeners) listener()
  }

  url(): string {
    return this.wc.getURL()
  }

  title(): string {
    return this.wc.getTitle()
  }

  isLoading(): boolean {
    return this.wc.isLoading()
  }

  private async answerDialog(params: unknown): Promise<void> {
    const dialog = params as { type: string; message: string }
    // alert and beforeunload cannot be refused usefully; confirm and prompt are dismissed, not accepted.
    this.capture.handle('Log.entryAdded', {
      entry: { level: 'info', text: `dialog ${dialog.type}: ${dialog.message}` }
    })
    const accept = dialog.type === 'alert' || dialog.type === 'beforeunload'
    await this.cdp.send('Page.handleJavaScriptDialog', { accept }).catch(() => undefined)
  }

  async waitLoaded(label: string): Promise<void> {
    const deadline = Date.now() + LOAD_TIMEOUT_MS
    await sleep(50)
    while (this.wc.isLoading()) {
      if (Date.now() > deadline)
        throw new WebError(
          `${label}: the page was still loading after ${LOAD_TIMEOUT_MS / 1000}s (${this.wc.getURL()})`
        )
      await sleep(50)
    }
  }

  async goto(url: string, label = 'open'): Promise<void> {
    await this.open()
    if (!isHttpUrl(url)) throw new WebError(`${label}: ${url} is not an http or https address`)
    const result = await this.cdp.send<{ errorText?: string }>('Page.navigate', { url })
    if (result.errorText) throw new WebError(`${label}: could not load ${url}: ${result.errorText}`)
    await this.waitLoaded(label)
  }

  async back(): Promise<void> {
    const history = this.wc.navigationHistory
    if (!history.canGoBack()) throw new WebError('back: there is no earlier page in this history')
    history.goBack()
    await this.waitLoaded('back')
  }

  async reload(): Promise<void> {
    this.wc.reload()
    await this.waitLoaded('reload')
  }

  async evaluate<T>(expression: string): Promise<T> {
    const result = await this.cdp.send<{
      result: { value?: T }
      exceptionDetails?: { text: string }
    }>('Runtime.evaluate', { expression, returnByValue: true })
    if (result.exceptionDetails)
      throw new WebError(`page script failed: ${result.exceptionDetails.text}`)
    return result.result.value as T
  }

  async passwordFieldVisible(): Promise<boolean> {
    return this.evaluate<boolean>(
      `Array.from(document.querySelectorAll('input[type=password]')).some(e => { const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0 })`
    )
  }

  // Milliseconds since the document last changed; zero when the page cannot be asked, for example mid navigation.
  async quietMs(): Promise<number> {
    try {
      return await this.evaluate<number>('performance.now() - window.__lgrassTick')
    } catch {
      return 0
    }
  }

  async snapshot(): Promise<string> {
    await this.open()
    const { nodes } = await this.cdp.send<{ nodes: AXNode[] }>('Accessibility.getFullAXTree')
    const masked = new Set<number>()
    for (const node of nodes) {
      const role = String((node.role?.value as string | undefined) ?? '')
      if (node.backendDOMNodeId === undefined) continue
      if (role !== 'textbox' && role !== 'searchbox' && role !== 'combobox') continue
      const described = await this.cdp
        .send<{ node: { attributes?: string[] } }>('DOM.describeNode', {
          backendNodeId: node.backendDOMNodeId
        })
        .catch(() => undefined)
      const attrs = described?.node.attributes ?? []
      if (isSecretInput(attrs)) masked.add(node.backendDOMNodeId)
    }
    const derived = await deriveNames(this.cdp, nodes)
    const clickable = await findClickable(this.cdp)
    const result = formatSnapshot(nodes, {
      url: this.url(),
      title: this.title(),
      firstRef: this.nextRef,
      masked,
      derived,
      clickable
    })
    this.firstLiveRef = this.nextRef
    this.nextRef = result.nextRef
    this.refs = new Map(result.refs.map((r) => [r.ref, r]))
    return result.text
  }

  refTarget(ref: string): SnapshotRef {
    const match = /^e(\d+)$/.exec(ref)
    if (!match)
      throw new WebError(`${ref} is not a ref; refs look like e12 and come from a snapshot`)
    const found = this.refs.get(ref)
    if (found) return found
    if (Number(match[1]) < this.firstLiveRef)
      throw new WebError(`${ref} is from an older snapshot; run snapshot again and use its refs`)
    throw new WebError(`${ref} is not in the latest snapshot; run snapshot again`)
  }

  async screenshot(): Promise<string> {
    await this.open()
    if (await this.passwordFieldVisible())
      throw new WebError('screenshot: not taken because the page shows a password field')
    // capturePage wakes a hidden view for the capture, which Page.captureScreenshot does not.
    const shot = this.wc.capturePage().then((image) => image.toPNG().toString('base64'))
    const timeout = sleep(SCREENSHOT_TIMEOUT_MS).then(() => undefined)
    const data = await Promise.race([shot, timeout])
    if (!data)
      throw new WebError(
        `screenshot: the page did not render within ${SCREENSHOT_TIMEOUT_MS / 1000}s`
      )
    return data
  }

  destroy(): void {
    try {
      this.wc.debugger.detach()
    } catch {
      // Already detached when the contents closed first.
    }
    if (!this.wc.isDestroyed()) this.wc.close()
  }
}
