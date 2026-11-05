// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app, BrowserWindow, dialog, ipcMain } from 'electron'
import { createWriteStream, unlinkSync } from 'fs'
import type { WriteStream } from 'fs'
import { homedir } from 'os'
import { join } from 'path'
import type {
  ResterClientCall,
  ResterClientDownload,
  ResterClientRequest,
  ResterClientResult,
  ResterClientRun
} from '../preload/types'
import { DownloadDecoder, type DownloadEvent } from './downloadFrames'
import {
  appendRun,
  clearHistory,
  deleteRun,
  loadHistory,
  loadRequests,
  saveRequests
} from './resterClientStore'
import { ipcConnect } from './ipcAddress'
import { vaultCall } from './vault'

const STORE_DIR = join(homedir(), '.lemongrass', 'rester-client')
// Where the first build kept one list per project.
const LEGACY_STORE_FILE = join(homedir(), '.lemongrass', 'rester-client.json')

interface WireResult {
  status: number
  headers: Record<string, string[]> | null
  body: string
  binary: boolean
  size: number
  truncated: boolean
  duration_ms: number
  url: string
}

// The inline answer a download gives for a non-2xx response: restergate.HTTPResult, whose body
// is parsed JSON or a string.
interface WireInline {
  status: number
  headers: Record<string, string[]> | null
  body: unknown
  url: string
}

interface WireHeader {
  ok: boolean
  error?: string
  payload?: { inline?: WireInline; stream?: { status: number; name: string } }
}

// The vault takes a []byte body as base64 and vault.Header with Go's field names.
function wireCall(call: ResterClientCall): unknown {
  return {
    rester: call.rester,
    environment: call.environment,
    user: call.user,
    method: call.method,
    path: call.path,
    body: call.body ? Buffer.from(call.body, 'utf-8').toString('base64') : undefined,
    content_type: call.contentType || undefined,
    headers: call.headers.map((header) => ({ Name: header.name, Value: header.value }))
  }
}

function fromWire(result: WireResult): ResterClientResult {
  return {
    status: result.status,
    headers: result.headers ?? {},
    body: result.body,
    binary: result.binary,
    size: result.size,
    truncated: result.truncated,
    durationMs: result.duration_ms,
    url: result.url
  }
}

function fromInline(inline: WireInline, durationMs: number): ResterClientResult {
  const body =
    inline.body === null || inline.body === undefined
      ? ''
      : typeof inline.body === 'string'
        ? inline.body
        : JSON.stringify(inline.body)
  return {
    status: inline.status,
    headers: inline.headers ?? {},
    body,
    binary: false,
    size: Buffer.byteLength(body),
    truncated: false,
    durationMs,
    url: inline.url
  }
}

async function humanRequest(
  passphrase: string,
  call: ResterClientCall
): Promise<ResterClientResult> {
  const result = await vaultCall<WireResult>('human_request', {
    root_secret: passphrase,
    call: wireCall(call)
  })
  return fromWire(result)
}

async function humanFlush(rester: string, environment: string, user: string): Promise<number> {
  const { flushed } = await vaultCall<{ flushed: number }>('human_flush', {
    rester,
    environment,
    user
  })
  return flushed
}

// Streams a 2xx body to a file the human picks once the response has started, so the dialog can
// offer the server's file name. The socket is paused while the dialog is open; the vault gives up
// on a write after restergate.RequestTimeout, which bounds how long the dialog can stay open.
async function humanDownload(
  win: BrowserWindow | null,
  passphrase: string,
  call: ResterClientCall
): Promise<ResterClientDownload> {
  const socket = await ipcConnect('vault')
  return new Promise((resolve, reject) => {
    const start = Date.now()
    const decoder = new DownloadDecoder()
    const queued: DownloadEvent[] = []
    let file: WriteStream | null = null
    let filePath = ''
    let status = 0
    let bytes = 0
    let choosing = false
    let closed = false
    let settled = false

    const settle = (fn: () => void): void => {
      if (settled) return
      settled = true
      socket.destroy()
      fn()
    }
    const fail = (err: Error): void =>
      settle(() => {
        if (file) {
          file.destroy()
          try {
            unlinkSync(filePath)
          } catch {
            // Nothing was written yet.
          }
        }
        reject(err)
      })

    const chooseFile = async (name: string): Promise<void> => {
      choosing = true
      socket.pause()
      const options = { defaultPath: join(app.getPath('downloads'), name || 'download') }
      const picked = win
        ? await dialog.showSaveDialog(win, options)
        : await dialog.showSaveDialog(options)
      if (settled) return
      if (picked.canceled || !picked.filePath) {
        settle(() => resolve({ kind: 'cancelled' }))
        return
      }
      filePath = picked.filePath
      file = createWriteStream(filePath)
      file.on('error', fail)
      choosing = false
      for (const event of queued.splice(0)) handle(event)
      if (settled || file.writableEnded) return
      if (closed) fail(new Error('lgrass: the download stopped while the save dialog was open'))
      else socket.resume()
    }

    const handle = (event: DownloadEvent): void => {
      if (settled) return
      if (choosing) {
        queued.push(event)
        return
      }
      switch (event.kind) {
        case 'header': {
          const line = event.line as WireHeader
          if (!line.ok) {
            fail(new Error(line.error || 'lgrass: human_download failed'))
          } else if (line.payload?.inline) {
            const result = fromInline(line.payload.inline, Date.now() - start)
            settle(() => resolve({ kind: 'inline', result }))
          } else if (line.payload?.stream) {
            status = line.payload.stream.status
            void chooseFile(line.payload.stream.name)
          }
          return
        }
        case 'data':
          bytes += event.data.length
          if (!file!.write(event.data)) {
            socket.pause()
            file!.once('drain', () => socket.resume())
          }
          return
        case 'end':
          file!.end(() =>
            settle(() =>
              resolve({
                kind: 'saved',
                path: filePath,
                bytes,
                status,
                durationMs: Date.now() - start
              })
            )
          )
          return
        case 'abort':
          fail(new Error(event.message))
      }
    }

    socket.write(
      JSON.stringify({
        op: 'human_download',
        payload: { root_secret: passphrase, call: wireCall(call) }
      }) + '\n'
    )
    socket.on('data', (chunk: Buffer) => {
      try {
        for (const event of decoder.push(chunk)) handle(event)
      } catch (err) {
        fail(err as Error)
      }
    })
    socket.on('error', fail)
    socket.on('close', () => {
      closed = true
      if (!choosing && !file?.writableEnded)
        fail(new Error('lgrass: the download stopped before its end'))
    })
  })
}

export function registerResterClientHandlers(): void {
  ipcMain.handle('resterClient:request', (_event, passphrase: string, call: ResterClientCall) =>
    humanRequest(passphrase, call)
  )
  ipcMain.handle('resterClient:download', (event, passphrase: string, call: ResterClientCall) =>
    humanDownload(BrowserWindow.fromWebContents(event.sender), passphrase, call)
  )
  ipcMain.handle(
    'resterClient:flush',
    (_event, rester: string, environment: string, user: string) =>
      humanFlush(rester, environment, user)
  )
  ipcMain.handle('resterClient:load', () => loadRequests(STORE_DIR, LEGACY_STORE_FILE))
  ipcMain.handle('resterClient:save', (_event, requests: ResterClientRequest[]) =>
    saveRequests(STORE_DIR, requests)
  )
  ipcMain.handle('resterClient:history', (_event, requestId: string) =>
    loadHistory(STORE_DIR, requestId)
  )
  ipcMain.handle('resterClient:recordRun', (_event, requestId: string, run: ResterClientRun) =>
    appendRun(STORE_DIR, requestId, run)
  )
  ipcMain.handle('resterClient:deleteRun', (_event, requestId: string, runId: string) =>
    deleteRun(STORE_DIR, requestId, runId)
  )
  ipcMain.handle('resterClient:clearHistory', (_event, requestId: string) =>
    clearHistory(STORE_DIR, requestId)
  )
}
