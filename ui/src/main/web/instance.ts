// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { randomUUID } from 'crypto'
import { BrowserWindow } from 'electron'
import { Downloads } from './download'
import { Page } from './page'
import type { RedactHints } from './types'
import type { WebBounds, WebState } from '../../preload/types'

// Pages that no pane shows still need a window to render and run animation frames, so they live in a
// window nobody sees until a pane takes the view.
let holder: BrowserWindow | undefined

export function holderWindow(): BrowserWindow {
  if (!holder || holder.isDestroyed()) {
    holder = new BrowserWindow({
      show: false,
      width: 1280,
      height: 800,
      webPreferences: { backgroundThrottling: false }
    })
  }
  return holder
}

export function closeHolder(): void {
  if (holder && !holder.isDestroyed()) holder.destroy()
  holder = undefined
}

export function isHolder(win: BrowserWindow): boolean {
  return win === holder
}

interface Host {
  window: BrowserWindow
  bounds: WebBounds
}

// Where a tab's pane wants the browser drawn. It is kept apart from the instance so a pane can open first.
const hosts = new Map<string, Host>()

export function setHost(tabId: string, host: Host): void {
  hosts.set(tabId, host)
}

export function updateHostBounds(tabId: string, bounds: WebBounds): Host | undefined {
  const host = hosts.get(tabId)
  if (host) host.bounds = bounds
  return host
}

export function dropHost(tabId: string): Host | undefined {
  const host = hosts.get(tabId)
  hosts.delete(tabId)
  return host
}

export function hostOf(tabId: string): Host | undefined {
  return hosts.get(tabId)
}

export class Instance {
  readonly hints: RedactHints = { tokenPlacement: null, tokenPath: '' }
  readonly pages = new Map<string, Page>()
  readonly downloads = new WeakMap<Page, Downloads>()
  active: { user: string; page: Page } | undefined
  // Set when a switch to this user failed: nothing is active until a switch succeeds, so no action can run
  // as the user the page was on before.
  failedUser = ''
  // The channel's web origin, learned from the vault with the first user.
  origin = ''
  // Every login value this instance has been given, kept only so execute() can scrub them from what it returns.
  readonly secrets = new Set<string>()
  private queue: Promise<unknown> = Promise.resolve()

  constructor(
    readonly tabId: string,
    readonly channelId: string,
    private readonly notify: (state: WebState) => void
  ) {}

  // Commands of one instance run one at a time: two at once would race over the same page.
  run<T>(work: () => Promise<T>): Promise<T> {
    const next = this.queue.then(work, work)
    this.queue = next.catch(() => undefined)
    return next
  }

  state(): WebState {
    const page = this.active?.page
    return {
      tabId: this.tabId,
      url: page?.url() ?? '',
      title: page?.title() ?? '',
      user: this.active?.user ?? '',
      open: true
    }
  }

  emit(): void {
    this.notify(this.state())
  }

  newPage(): Page {
    const page = new Page(`lgweb-${randomUUID()}`, () => this.hints)
    holderWindow().contentView.addChildView(page.view)
    page.view.setBounds({ x: 0, y: 0, width: 1280, height: 800 })
    page.onChange(() => {
      if (this.active?.page === page) this.emit()
    })
    // Human input from the watch tab. before-input-event sees only real keyboard input, never keys sent over
    // CDP, while input-event sees every mouse press, the engine's own included, which capture subtracts.
    const wc = page.view.webContents
    wc.on('before-input-event', (_event, input) => {
      if (input.type === 'keyDown') page.capture.keyDown()
    })
    wc.on('input-event', (_event, input) => {
      if (input.type === 'mouseDown') page.capture.mouseDown()
    })
    const downloads = new Downloads(page.capture)
    this.downloads.set(page, downloads)
    wc.session.on('will-download', (_event, item) => downloads.handle(item))
    return page
  }

  // Makes a page the active one, and the one the tab's pane shows when it is attached.
  activate(user: string, page: Page): void {
    this.failedUser = ''
    const previous = this.active?.page
    this.active = { user, page }
    const host = hosts.get(this.tabId)
    if (previous && previous !== page) this.park(previous)
    if (host) this.show(page, host)
    this.emit()
  }

  // A failed switch leaves no active user, and the pane stops showing the previous user's page.
  deactivate(failedUser: string): void {
    const previous = this.active?.page
    this.active = undefined
    this.failedUser = failedUser
    if (previous) this.park(previous)
    this.emit()
  }

  show(page: Page, host: Host): void {
    this.park(page)
    host.window.contentView.addChildView(page.view)
    page.view.setBounds(host.bounds)
  }

  // Back to the holder window, where the view keeps rendering with nobody looking at it.
  park(page: Page): void {
    for (const win of BrowserWindow.getAllWindows()) {
      if (!win.isDestroyed()) win.contentView.removeChildView(page.view)
    }
    holderWindow().contentView.addChildView(page.view)
    page.view.setBounds({ x: 0, y: 0, width: 1280, height: 800 })
  }

  destroy(): void {
    for (const page of this.pages.values()) {
      for (const win of BrowserWindow.getAllWindows()) {
        if (!win.isDestroyed()) win.contentView.removeChildView(page.view)
      }
      page.destroy()
    }
    this.pages.clear()
    this.active = undefined
    this.notify({ tabId: this.tabId, url: '', title: '', user: '', open: false })
  }
}
