import { sleep, type Page } from './page'
import { notSettledText, settledText, settleVerdict } from './settle'
import { WebError } from './types'

const READY_TIMEOUT_MS = 5_000
const SETTLE_TIMEOUT_MS = 10_000
const SETTLE_QUIET_MS = 300
const WAIT_TIMEOUT_MS = 10_000

// Runs with the element as `this`. The checks are attached, visible, enabled and topmost; the two frame
// stability check is left out on purpose. A failure names the check, and `detail` says what was seen.
const READINESS = `function (kind) {
  const el = this
  if (!el.isConnected) return { fail: 'attached', detail: 'the element is no longer in the page' }
  el.scrollIntoView({ block: 'center', inline: 'center', behavior: 'instant' })
  const rect = el.getBoundingClientRect()
  const style = getComputedStyle(el)
  if (style.visibility === 'hidden' || style.display === 'none' || rect.width === 0 || rect.height === 0)
    return { fail: 'visible', detail: 'the element is hidden or has no size' }
  if (el.disabled || el.getAttribute('aria-disabled') === 'true' || el.closest('fieldset:disabled'))
    return { fail: 'enabled', detail: 'the element is disabled' }
  if (kind === 'type') {
    const plain = el.tagName === 'INPUT' && !['checkbox', 'radio', 'button', 'submit', 'reset', 'file', 'image', 'range', 'color'].includes(el.type)
    if (!(el.isContentEditable || plain || el.tagName === 'TEXTAREA'))
      return { fail: 'editable', detail: 'the element does not take text' }
    if (el.readOnly) return { fail: 'editable', detail: 'the element is read-only' }
  }
  const x = Math.min(Math.max(rect.left + rect.width / 2, 0), innerWidth - 1)
  const y = Math.min(Math.max(rect.top + rect.height / 2, 0), innerHeight - 1)
  let hit = document.elementFromPoint(x, y)
  const top = hit
  const labels = el.labels ? Array.from(el.labels) : []
  while (hit) {
    if (hit === el || el.contains(hit) || labels.some((l) => l === hit || l.contains(hit))) return { ok: true, x, y }
    const root = hit.getRootNode()
    hit = root instanceof ShadowRoot ? root.host : null
  }
  const name = top ? top.tagName.toLowerCase() + (top.id ? '#' + top.id : '') : 'nothing'
  return { fail: 'topmost', detail: 'another element covers it: ' + name }
}`

interface Readiness {
  ok?: boolean
  fail?: string
  detail?: string
  x?: number
  y?: number
}

async function objectOf(page: Page, backendNodeId: number): Promise<string | undefined> {
  try {
    const { object } = await page.cdp.send<{ object: { objectId?: string } }>('DOM.resolveNode', {
      backendNodeId
    })
    return object.objectId
  } catch {
    return undefined
  }
}

async function objectOfSelector(page: Page, selector: string): Promise<string | undefined> {
  const result = await page.cdp.send<{ result: { objectId?: string; subtype?: string } }>(
    'Runtime.evaluate',
    { expression: `document.querySelector(${JSON.stringify(selector)})` }
  )
  return result.result.subtype === 'node' ? result.result.objectId : undefined
}

export interface Target {
  label: string
  find: () => Promise<string | undefined>
}

export function refTarget(page: Page, ref: string): Target {
  const target = page.refTarget(ref)
  const what = target.name ? `${ref} (${target.role} "${target.name}")` : `${ref} (${target.role})`
  return { label: what, find: () => objectOf(page, target.backendNodeId) }
}

export function selectorTarget(page: Page, selector: string): Target {
  return { label: `"${selector}"`, find: () => objectOfSelector(page, selector) }
}

// Retries the readiness checks until they pass or the timeout ends; the error names the last check that failed.
async function ready(
  page: Page,
  target: Target,
  kind: 'click' | 'type',
  timeoutMs = READY_TIMEOUT_MS
): Promise<{ objectId: string; x: number; y: number }> {
  const deadline = Date.now() + timeoutMs
  let last = 'attached: the element was not found in the page'
  for (;;) {
    const objectId = await target.find()
    if (objectId) {
      const { result, exceptionDetails } = await page.cdp.send<{
        result: { value?: Readiness }
        exceptionDetails?: unknown
      }>('Runtime.callFunctionOn', {
        objectId,
        functionDeclaration: READINESS,
        arguments: [{ value: kind }],
        returnByValue: true
      })
      const check = result.value
      if (!exceptionDetails && check?.ok && check.x !== undefined && check.y !== undefined)
        return { objectId, x: check.x, y: check.y }
      if (check?.fail) last = `${check.fail}: ${check.detail}`
    }
    if (Date.now() >= deadline)
      throw new WebError(
        `${kind} ${target.label}: not ready after ${timeoutMs / 1000}s, the ${last}`
      )
    await sleep(100)
  }
}

async function mouse(page: Page, type: string, x: number, y: number): Promise<void> {
  await page.cdp.send('Input.dispatchMouseEvent', {
    type,
    x,
    y,
    button: type === 'mouseMoved' ? 'none' : 'left',
    clickCount: type === 'mouseMoved' ? 0 : 1
  })
}

export async function click(page: Page, target: Target): Promise<void> {
  const { x, y } = await ready(page, target, 'click')
  await mouse(page, 'mouseMoved', x, y)
  await mouse(page, 'mousePressed', x, y)
  await mouse(page, 'mouseReleased', x, y)
}

const KEYS: Record<string, { key: string; code: string; vk: number; text?: string }> = {
  enter: { key: 'Enter', code: 'Enter', vk: 13, text: '\r' },
  tab: { key: 'Tab', code: 'Tab', vk: 9 },
  escape: { key: 'Escape', code: 'Escape', vk: 27 }
}

export async function press(page: Page, name: string): Promise<void> {
  const spec = KEYS[name.toLowerCase()]
  if (!spec) throw new WebError(`press: ${name} is not supported, use Enter, Tab or Escape`)
  const base = { key: spec.key, code: spec.code, windowsVirtualKeyCode: spec.vk }
  await page.cdp.send('Input.dispatchKeyEvent', {
    ...base,
    type: spec.text ? 'keyDown' : 'rawKeyDown',
    text: spec.text
  })
  await page.cdp.send('Input.dispatchKeyEvent', { ...base, type: 'keyUp' })
}

// Replaces whatever the field held: select it all, then insert the text as one input.
export async function type(page: Page, target: Target, text: string): Promise<void> {
  const { objectId } = await ready(page, target, 'type')
  await page.cdp.send('Runtime.callFunctionOn', {
    objectId,
    functionDeclaration: `function () {
      this.focus()
      if (typeof this.select === 'function') this.select()
      else { const r = document.createRange(); r.selectNodeContents(this); const s = getSelection(); s.removeAllRanges(); s.addRange(r) }
    }`
  })
  if (text === '') {
    await page.cdp.send('Input.dispatchKeyEvent', {
      type: 'rawKeyDown',
      key: 'Backspace',
      code: 'Backspace',
      windowsVirtualKeyCode: 8
    })
    await page.cdp.send('Input.dispatchKeyEvent', {
      type: 'keyUp',
      key: 'Backspace',
      code: 'Backspace',
      windowsVirtualKeyCode: 8
    })
    return
  }
  await page.cdp.send('Input.insertText', { text })
}

// The wait after an action: no foreground request in flight, the page not loading and the DOM quiet for a
// moment, or the cap. Requests open for over two seconds count as background. The answer says which, so a slow
// page is not mistaken for a broken one.
export async function settle(page: Page): Promise<string> {
  const started = Date.now()
  await sleep(100)
  for (;;) {
    const input = {
      loading: page.isLoading(),
      ages: page.capture.inFlightAges(),
      quiet: await page.quietMs(),
      quietNeeded: SETTLE_QUIET_MS
    }
    const verdict = settleVerdict(input)
    if (verdict.settled) return settledText(verdict)
    if (Date.now() - started >= SETTLE_TIMEOUT_MS)
      return notSettledText(input, verdict, SETTLE_TIMEOUT_MS / 1000)
    await sleep(50)
  }
}

function globToRegExp(pattern: string): RegExp {
  const escaped = pattern.replace(/[.+?^${}()|[\]\\]/g, '\\$&').replace(/\*/g, '.*')
  return new RegExp(`^${escaped}$`)
}

export function urlMatches(pattern: string, url: string): boolean {
  const re = globToRegExp(pattern)
  let short = url
  try {
    const parsed = new URL(url)
    short = parsed.pathname + parsed.search
  } catch {
    // A page without a parsable address is only matched whole.
  }
  return re.test(url) || re.test(short)
}

async function poll(condition: () => Promise<boolean>, timeoutMs: number): Promise<boolean> {
  const deadline = Date.now() + timeoutMs
  for (;;) {
    if (await condition()) return true
    if (Date.now() >= deadline) return false
    await sleep(100)
  }
}

export async function waitText(page: Page, text: string): Promise<void> {
  const present = (): Promise<boolean> =>
    page.evaluate<boolean>(
      `(document.body ? document.body.innerText : '').includes(${JSON.stringify(text)})`
    )
  if (!(await poll(present, WAIT_TIMEOUT_MS)))
    throw new WebError(
      `wait: the text "${text}" did not appear within ${WAIT_TIMEOUT_MS / 1000}s (page: ${page.url()})`
    )
}

export async function waitUrl(page: Page, pattern: string): Promise<void> {
  if (!(await poll(async () => urlMatches(pattern, page.url()), WAIT_TIMEOUT_MS)))
    throw new WebError(
      `wait: the address did not match ${pattern} within ${WAIT_TIMEOUT_MS / 1000}s (page: ${page.url()})`
    )
}

export async function waitGone(page: Page, ref: string): Promise<void> {
  const target = refTarget(page, ref)
  const gone = async (): Promise<boolean> => {
    const objectId = await target.find()
    if (!objectId) return true
    const { result } = await page.cdp.send<{ result: { value?: boolean } }>(
      'Runtime.callFunctionOn',
      {
        objectId,
        functionDeclaration: `function () { if (!this.isConnected) return true; const r = this.getBoundingClientRect(); const s = getComputedStyle(this); return s.display === 'none' || s.visibility === 'hidden' || (r.width === 0 && r.height === 0) }`,
        returnByValue: true
      }
    )
    return result.value === true
  }
  if (!(await poll(gone, WAIT_TIMEOUT_MS)))
    throw new WebError(
      `wait: ${target.label} was still in the page after ${WAIT_TIMEOUT_MS / 1000}s`
    )
}

export async function waitSelector(page: Page, selector: string, timeoutMs: number): Promise<void> {
  const target = selectorTarget(page, selector)
  const found = (): Promise<boolean> => target.find().then((id) => id !== undefined)
  if (!(await poll(found, timeoutMs)))
    throw new WebError(
      `wait: ${target.label} did not appear within ${Math.round(timeoutMs / 1000)}s (page: ${page.url()})`
    )
}
