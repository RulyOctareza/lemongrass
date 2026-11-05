// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { ipcMain, WebContents } from 'electron'
import * as net from 'net'
import type { AskTermAttach } from '../preload/types'
import { ipcConnect } from './ipcAddress'

// A command ask's terminal lives in the vault daemon (gatekeeper/askattach.go). attach_ask keeps
// its connection open, unlike every other vault op: the first JSON line is the reply with the
// replay, then the daemon sends output frames and one end frame, and this side sends input and
// resize frames. Data is base64 in both directions.

interface AttachReply {
  ok: boolean
  error?: string
  payload?: { replay?: string }
}

interface Frame {
  type: string
  data?: string
  reason?: string
}

const attachments = new Map<number, net.Socket>()
let nextId = 1

function sendTo(sender: WebContents, channel: string, payload: unknown): void {
  if (!sender.isDestroyed()) sender.send(channel, payload)
}

function writeFrame(socket: net.Socket, frame: object): void {
  if (!socket.destroyed) socket.write(JSON.stringify(frame) + '\n')
}

async function attach(sender: WebContents, passphrase: string, id: string): Promise<AskTermAttach> {
  const socket = await ipcConnect('vault')
  return new Promise((resolve, reject) => {
    const attachId = nextId++
    let buffered = ''
    let replied = false
    let ended = false

    const end = (reason: string): void => {
      if (ended) return
      ended = true
      attachments.delete(attachId)
      sendTo(sender, 'ask:term-end', { attachId, reason })
    }

    const handleLine = (line: string): void => {
      if (!replied) {
        replied = true
        const reply = JSON.parse(line) as AttachReply
        if (!reply.ok) {
          socket.destroy()
          reject(new Error(reply.error || 'lgrass: attach_ask failed'))
          return
        }
        attachments.set(attachId, socket)
        resolve({ attachId, replay: Buffer.from(reply.payload?.replay ?? '', 'base64') })
        return
      }
      const frame = JSON.parse(line) as Frame
      if (frame.type === 'output' && frame.data) {
        sendTo(sender, 'ask:term-data', { attachId, data: Buffer.from(frame.data, 'base64') })
      } else if (frame.type === 'end') {
        end(frame.reason ?? 'ended')
        socket.end()
      }
    }

    // A closed window takes its views with it, so the daemon frees the terminal for the next view.
    const onDestroyed = (): void => {
      socket.destroy()
    }
    sender.once('destroyed', onDestroyed)

    socket.setEncoding('utf-8')
    socket.write(
      JSON.stringify({ op: 'attach_ask', payload: { root_secret: passphrase, id } }) + '\n'
    )
    socket.on('data', (chunk: string) => {
      buffered += chunk
      let newline = buffered.indexOf('\n')
      while (newline >= 0) {
        const line = buffered.slice(0, newline)
        buffered = buffered.slice(newline + 1)
        if (line.trim()) {
          try {
            handleLine(line)
          } catch (err) {
            socket.destroy()
            if (!replied) reject(err)
            return
          }
        }
        newline = buffered.indexOf('\n')
      }
    })
    socket.on('error', (err) => {
      if (!replied) reject(err)
    })
    socket.on('close', () => {
      if (!sender.isDestroyed()) sender.removeListener('destroyed', onDestroyed)
      if (!replied) {
        reject(new Error('lgrass: no response from attach_ask'))
        return
      }
      end('detached')
    })
  })
}

export function registerAskTerminal(): void {
  ipcMain.handle('ask:term-attach', (event, passphrase: string, id: string) =>
    attach(event.sender, passphrase, id)
  )
  ipcMain.on('ask:term-input', (_event, { attachId, data }: { attachId: number; data: string }) => {
    const socket = attachments.get(attachId)
    if (socket)
      writeFrame(socket, { type: 'input', data: Buffer.from(data, 'utf-8').toString('base64') })
  })
  ipcMain.on(
    'ask:term-resize',
    (_event, { attachId, rows, cols }: { attachId: number; rows: number; cols: number }) => {
      const socket = attachments.get(attachId)
      if (socket) writeFrame(socket, { type: 'resize', rows, cols })
    }
  )
  ipcMain.on('ask:term-detach', (_event, { attachId }: { attachId: number }) => {
    attachments.get(attachId)?.destroy()
  })
}
