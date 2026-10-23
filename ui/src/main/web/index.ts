import { BrowserWindow, ipcMain } from 'electron'
import type { WebBounds, WebState } from '../../preload/types'
import { vaultCall } from '../vault'
import { accessSync, constants, statSync } from 'fs'
import { basename, isAbsolute } from 'path'
import {
  click,
  press,
  refTarget,
  settle,
  type,
  upload,
  waitGone,
  waitText,
  waitUrl
} from './actions'
import { runLogin } from './login'
import { scrubText, secretFieldValues } from './redact'
import {
  Instance,
  closeHolder,
  dropHost,
  holderWindow,
  isHolder,
  setHost,
  updateHostBounds
} from './instance'
import { isHttpUrl, type Page } from './page'
import { WebError, type WebFlags, type WebLoginFields, type WebResult } from './types'

const instances = new Map<string, Instance>()

function broadcast(state: WebState): void {
  for (const win of BrowserWindow.getAllWindows()) {
    if (!win.isDestroyed() && !isHolder(win)) win.webContents.send('web:state', state)
  }
}

function closedState(tabId: string): WebState {
  return { tabId, url: '', title: '', user: '', open: false }
}

export function getState(tabId: string): WebState {
  return instances.get(tabId)?.state() ?? closedState(tabId)
}

export function attachView(tabId: string, window: BrowserWindow, bounds: WebBounds): void {
  setHost(tabId, { window, bounds })
  const active = instances.get(tabId)?.active
  if (active) instances.get(tabId)?.show(active.page, { window, bounds })
}

export function setBounds(tabId: string, bounds: WebBounds): void {
  const host = updateHostBounds(tabId, bounds)
  const active = instances.get(tabId)?.active
  if (host && active) active.page.view.setBounds(bounds)
}

export function detachView(tabId: string): void {
  if (!dropHost(tabId)) return
  const active = instances.get(tabId)?.active
  if (active) instances.get(tabId)?.park(active.page)
}

// Ends a tab's browser: its pages, their sessions and their views. Safe to call for a tab with none.
export function destroyInstance(tabId: string): void {
  instances.get(tabId)?.destroy()
  instances.delete(tabId)
}

// The holder window counts as a window, so it must go too or the app would not quit when the main one closes.
export function destroyAllInstances(): void {
  for (const tabId of [...instances.keys()]) destroyInstance(tabId)
  closeHolder()
}

export function registerWebHandlers(): void {
  ipcMain.handle('web:attach', (event, tabId: string, bounds: WebBounds) => {
    const window = BrowserWindow.fromWebContents(event.sender)
    if (window) attachView(tabId, window, bounds)
  })
  ipcMain.handle('web:set-bounds', (_event, tabId: string, bounds: WebBounds) =>
    setBounds(tabId, bounds)
  )
  ipcMain.handle('web:detach', (_event, tabId: string) => detachView(tabId))
  ipcMain.handle('web:get-state', (_event, tabId: string) => getState(tabId))
}

function instanceFor(tabId: string, channelId: string): Instance {
  const existing = instances.get(tabId)
  if (existing && existing.channelId === channelId) return existing
  // A tab that switches to another channel starts a clean browser: the old pages belong to the old rester.
  existing?.destroy()
  const created = new Instance(tabId, channelId, broadcast)
  instances.set(tabId, created)
  return created
}

// The page for a user, logging in when that user has none yet. The vault reply is the only place the login
// values exist, and only runLogin reads them.
async function pageFor(inst: Instance, requested: string): Promise<Page> {
  if (requested === '' && inst.active) return inst.active.page
  if (requested === '' && inst.failedUser) throw new WebError(noUserText(inst))
  try {
    return await switchTo(inst, requested)
  } catch (err) {
    // An explicit switch that fails must not leave the previous user acting under a name the model no longer means.
    if (requested !== '') inst.deactivate(requested)
    throw err
  }
}

function noUserText(inst: Instance): string {
  return `the last switch to ${inst.failedUser} failed, so no user is active: run user <name> or open <path> --user <name> first`
}

async function switchTo(inst: Instance, requested: string): Promise<Page> {
  const login = await vaultCall<WebLoginFields>('web_login_fields', {
    channel_id: inst.channelId,
    user: requested
  })
  inst.hints.tokenPlacement = login.token_placement?.Name ? login.token_placement : null
  inst.hints.tokenPath = login.token_path ?? ''
  const user = login.user || requested || 'user'
  inst.origin = login.origin
  for (const value of secretFieldValues(login.fields ?? {}, [user, requested]))
    inst.secrets.add(value)
  const existing = inst.pages.get(user)
  if (existing) {
    inst.activate(user, existing)
    return existing
  }
  const page = inst.newPage()
  await page.open()
  inst.pages.set(user, page)
  try {
    await runLogin(page, login, user)
  } catch (err) {
    inst.pages.delete(user)
    page.destroy()
    throw new WebError(`${err instanceof Error ? err.message : String(err)}\nno user is active now`)
  }
  // The login's own requests are dropped, so no credential sent or returned there can ever be listed.
  page.capture.clear()
  inst.activate(user, page)
  return page
}

function activePage(inst: Instance): Page {
  if (!inst.active)
    throw new WebError(inst.failedUser ? noUserText(inst) : 'no page is open yet: run open first')
  return inst.active.page
}

function resolveUrl(inst: Instance, target: string): string {
  if (isHttpUrl(target)) return target
  if (!inst.origin)
    throw new WebError(`open: ${target} needs a full address, the channel has no origin`)
  const url = new URL(target, inst.origin).toString()
  if (!isHttpUrl(url)) throw new WebError(`open: ${target} is not an http or https address`)
  return url
}

// What an action returns: what it did, how the page settled, counts of the requests and console errors it
// caused, and what changed on the page. The requests themselves are listed only when the model asks with network.
async function report(inst: Instance, page: Page, label: string, action: number): Promise<string> {
  const note = await settle(page)
  await page.capture.settled()
  const snapshot = await page.actionSnapshot()
  return [
    `as ${inst.active?.user ?? ''}`,
    `action ${action}: ${label}`,
    page.capture.takeHumanSummary(),
    page.capture.takeDownloadNotes(),
    note,
    page.capture.actionSummary(action),
    snapshot
  ]
    .filter((part) => part !== '')
    .join('\n')
}

// A request id as network prints it, r12, or the bare number. #12 is taken too, for a quoted copy.
function requestId(arg: string): number {
  const match = /^[r#]?(\d+)$/i.exec(arg.trim())
  if (!match) throw new WebError(`network: ${arg} is not a request id, give it as r12`)
  return Number(match[1])
}

// The CLI checked the files already; the app checks again, since it is the one that hands them to the page.
function checkUploadFile(file: string): void {
  if (!isAbsolute(file)) throw new WebError(`upload: ${file} is not an absolute path`)
  let regular = false
  try {
    regular = statSync(file).isFile()
    accessSync(file, constants.R_OK)
  } catch {
    throw new WebError(`upload: ${file} does not exist or cannot be read`)
  }
  if (!regular) throw new WebError(`upload: ${file} is not a regular file`)
}

function needArgs(command: string, args: string[], count: number, usage: string): void {
  if (args.length < count) throw new WebError(`${command}: missing argument, usage: ${usage}`)
}

async function run(
  inst: Instance,
  command: string,
  args: string[],
  flags: WebFlags
): Promise<WebResult> {
  const user = flags.user ?? ''
  if (command === 'open') {
    needArgs(command, args, 1, 'open <path-or-url> [--user <name>]')
    const page = await pageFor(inst, user)
    const action = page.capture.startAction()
    const url = resolveUrl(inst, args[0])
    await page.goto(url)
    return { text: await report(inst, page, `opened ${page.url()}`, action) }
  }
  if (command === 'user') {
    needArgs(command, args, 1, 'user <name>')
    const page = await pageFor(inst, args[0])
    page.capture.startAction()
    return {
      text: [`active user: ${inst.active?.user ?? args[0]}`, await page.snapshot()].join('\n')
    }
  }
  const page = activePage(inst)
  if (command === 'snapshot')
    return {
      text: [
        `as ${inst.active?.user ?? ''}`,
        page.capture.takeHumanSummary(),
        page.capture.takeDownloadNotes(),
        await page.snapshot({ within: flags.within || undefined })
      ]
        .filter((part) => part !== '')
        .join('\n')
    }
  if (command === 'network') {
    await page.capture.settled()
    const since = flags.since === undefined || flags.since === '' ? undefined : Number(flags.since)
    if (since !== undefined && !Number.isInteger(since))
      throw new WebError('network: --since takes an action number')
    if (args.length > 0) {
      const id = requestId(args[0])
      const text = page.capture.request(id)
      if (text === undefined)
        throw new WebError(`network: there is no request r${id}, run network to list them`)
      return { text }
    }
    return { text: page.capture.network({ failed: flags.failed === 'true', since }) }
  }
  if (command === 'console')
    return { text: page.capture.consoleLog({ errors: flags.errors === 'true' }) }
  if (command === 'screenshot') return { text: 'screenshot taken', data: await page.screenshot() }
  const action = page.capture.startAction()
  if (command === 'back') {
    await page.back()
    return { text: await report(inst, page, `went back to ${page.url()}`, action) }
  }
  if (command === 'reload') {
    await page.reload()
    return { text: await report(inst, page, `reloaded ${page.url()}`, action) }
  }
  if (command === 'click') {
    needArgs(command, args, 1, 'click <ref>')
    await page.verifyRef(args[0])
    const target = refTarget(page, args[0])
    await click(page, target)
    return { text: await report(inst, page, `clicked ${target.label}`, action) }
  }
  if (command === 'type') {
    needArgs(command, args, 2, 'type <ref> <text> [--submit]')
    await page.verifyRef(args[0])
    const target = refTarget(page, args[0])
    await type(page, target, args.slice(1).join(' '))
    if (flags.submit === 'true') await press(page, 'Enter')
    const submitted = flags.submit === 'true' ? ' and pressed Enter' : ''
    return { text: await report(inst, page, `typed into ${target.label}${submitted}`, action) }
  }
  if (command === 'press') {
    needArgs(command, args, 1, 'press <key>')
    await press(page, args[0])
    return { text: await report(inst, page, `pressed ${args[0]}`, action) }
  }
  if (command === 'upload') {
    needArgs(command, args, 2, 'upload <ref> <file>...')
    const files = args.slice(1)
    for (const file of files) checkUploadFile(file)
    const { backendNodeId } = await page.verifyRef(args[0])
    const target = refTarget(page, args[0])
    await upload(page, target, backendNodeId, files)
    const names = files.map((f) => basename(f)).join(', ')
    return { text: await report(inst, page, `uploaded ${names} to ${target.label}`, action) }
  }
  if (command === 'download') {
    needArgs(command, args, 1, 'download <ref> --out <file-or-directory>')
    await page.verifyRef(args[0])
    const target = refTarget(page, args[0])
    const downloads = inst.downloads.get(page)
    if (!downloads) throw new WebError('download: this page cannot download')
    const got = await downloads.catchDownload(() => click(page, target), target.label)
    const label = `downloaded ${got.name} (${got.data.length} bytes) by clicking ${target.label}`
    return {
      text: await report(inst, page, label, action),
      data: got.data.toString('base64'),
      name: got.name
    }
  }
  if (command === 'wait') {
    if (flags.text) await waitText(page, flags.text)
    else if (flags.gone) await waitGone(page, flags.gone)
    else if (flags.url) await waitUrl(page, flags.url)
    else throw new WebError('wait: give --text <text>, --gone <ref> or --url <pattern>')
    return { text: await report(inst, page, 'wait condition met', action) }
  }
  throw new WebError(`unknown web command ${command}`)
}

export async function execute(
  tabId: string,
  channelId: string,
  command: string,
  args: string[],
  flags: WebFlags
): Promise<WebResult> {
  if (tabId === '') throw new WebError('web commands only work inside a lemongrass tab')
  holderWindow()
  const inst = instanceFor(tabId, channelId)
  // The one scrub: whatever the command built, success or failure, passes through it before it leaves.
  const scrub = (text: string): string => scrubText(text, inst.secrets, inst.hints)
  try {
    const result = await inst.run(() => run(inst, command, args, flags))
    return {
      text: scrub(result.text),
      data: result.data,
      name: result.name === undefined ? undefined : scrub(result.name)
    }
  } catch (err) {
    throw new WebError(scrub(err instanceof Error ? err.message : String(err)))
  }
}

interface WebRequest {
  tab_id?: string
  channel_id?: string
  command?: string
  args?: string[]
  flags?: WebFlags
}

// The app socket's `web` op. Any failure becomes the model-facing error text.
export async function handleWebOp(
  request: WebRequest
): Promise<{ ok: boolean; text?: string; data?: string; name?: string; error?: string }> {
  try {
    const result = await execute(
      String(request.tab_id ?? ''),
      String(request.channel_id ?? ''),
      String(request.command ?? ''),
      (request.args ?? []).map(String),
      request.flags ?? {}
    )
    return { ok: true, text: result.text, data: result.data, name: result.name }
  } catch (err) {
    return { ok: false, error: err instanceof Error ? err.message : String(err) }
  }
}
