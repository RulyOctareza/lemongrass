// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { mkdtempSync, readFileSync, rmSync } from 'fs'
import { tmpdir } from 'os'
import { join } from 'path'
import type { DownloadItem } from 'electron'
import type { Capture } from './capture'
import { WebError } from './types'

const START_TIMEOUT_MS = 10_000
// Below the 60 s deadline of one web command, so a slow download fails here with a reason, not at the socket.
const DONE_TIMEOUT_MS = 40_000
// The bytes travel back to the CLI in one reply, base64 encoded.
const SIZE_CAP = 50 * 1024 * 1024

export interface Downloaded {
  name: string
  data: Buffer
}

// The downloads of one page. Only a download the model asked for with `download` is saved; one that follows a
// model action unasked is cancelled before a save dialog can open, and one the human started from the watch
// tab keeps its normal save dialog.
export class Downloads {
  // Set while a download command waits for its file.
  private waiting: ((saving: { done: Promise<Downloaded> }) => void) | undefined

  constructor(private readonly capture: Capture) {}

  // The session's will-download handler. The save path must be set before it returns, or Electron asks the user.
  handle(item: DownloadItem): void {
    const waiter = this.waiting
    if (waiter) {
      this.waiting = undefined
      waiter({ done: save(item) })
      return
    }
    if (this.capture.humanInputSinceAction) return
    item.cancel()
    this.capture.noteCancelledDownload(item.getFilename(), item.getTotalBytes())
  }

  // Runs `start`, which should make the page download a file, and returns that file.
  async catchDownload(start: () => Promise<void>, label: string): Promise<Downloaded> {
    const arrived = new Promise<{ done: Promise<Downloaded> }>((resolve) => {
      this.waiting = resolve
    })
    try {
      await start()
      let timer: NodeJS.Timeout | undefined
      const timeout = new Promise<undefined>((resolve) => {
        timer = setTimeout(() => resolve(undefined), START_TIMEOUT_MS)
      })
      const saving = await Promise.race([arrived, timeout])
      clearTimeout(timer)
      if (!saving)
        throw new WebError(
          `download: ${label} started no download within ${START_TIMEOUT_MS / 1000}s`
        )
      return await saving.done
    } finally {
      this.waiting = undefined
    }
  }
}

function save(item: DownloadItem): Promise<Downloaded> {
  const dir = mkdtempSync(join(tmpdir(), 'lgrass-web-download-'))
  const path = join(dir, 'file')
  item.setSavePath(path)
  const name = item.getFilename()
  const tooBig = (): boolean =>
    item.getTotalBytes() > SIZE_CAP || item.getReceivedBytes() > SIZE_CAP
  let reason = ''
  const stop = (why: string): void => {
    reason = why
    item.cancel()
  }
  if (tooBig()) stop(`it is larger than ${SIZE_CAP / 1024 / 1024} MB`)
  item.on('updated', () => {
    if (!reason && tooBig()) stop(`it is larger than ${SIZE_CAP / 1024 / 1024} MB`)
  })
  const timer = setTimeout(
    () => stop(`it did not finish within ${DONE_TIMEOUT_MS / 1000}s`),
    DONE_TIMEOUT_MS
  )
  return new Promise((resolve, reject) => {
    item.once('done', (_event, state) => {
      clearTimeout(timer)
      try {
        if (state === 'completed') resolve({ name, data: readFileSync(path) })
        else
          reject(
            new WebError(
              `download: ${name} was not saved, ${reason || `the download was ${state}`}`
            )
          )
      } catch (err) {
        reject(err)
      } finally {
        rmSync(dir, { recursive: true, force: true })
      }
    })
  })
}
