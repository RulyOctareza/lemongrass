// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile } from 'child_process'
import { readFile } from 'fs/promises'
import * as net from 'net'
import { homedir } from 'os'
import { join } from 'path'

// Where the app reaches lgrassd's daemons and where it serves app.sock, mirroring internal/ipc: a unix socket under
// ~/.lemongrass on Linux and macOS, and on Windows the named pipe \\.\pipe\lemongrass-<sid>-<name>, since Node has
// no AF_UNIX there.

export type IpcName = 'vault' | 'agent' | 'node' | 'threads' | 'app'

const lemongrassDir = (): string => join(homedir(), '.lemongrass')

let sidPromise: Promise<string> | undefined

// whoami ships with every Windows since Vista and prints the SID the Go side reads from the process token.
function userSID(): Promise<string> {
  sidPromise ??= new Promise((resolve, reject) => {
    execFile('whoami', ['/user', '/fo', 'csv', '/nh'], { windowsHide: true }, (err, stdout) => {
      const sid = /"(S-1-[0-9-]+)"\s*$/.exec(stdout.trim())?.[1]
      if (err || !sid) {
        sidPromise = undefined
        reject(err ?? new Error('ipc: whoami printed no user SID'))
        return
      }
      resolve(sid)
    })
  })
  return sidPromise
}

export async function ipcAddress(name: IpcName): Promise<string> {
  if (process.platform !== 'win32') return join(lemongrassDir(), `${name}.sock`)
  return `\\\\.\\pipe\\lemongrass-${await userSID()}-${name}`
}

// The Go daemons on Windows speak first with "\0lgipc <token>\n", the token also kept in an owner-only file. Any user
// can create a pipe under our name before our daemon does, and Node cannot ask who owns a pipe, so a server that
// cannot show the token is dropped before a request, a passphrase included, is written to it.
const GREETING = '\0lgipc '
const GREETING_TIMEOUT_MS = 2000

export function verifyGreeting(
  socket: net.Socket,
  name: IpcName,
  keyDir = lemongrassDir()
): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    let buffered = Buffer.alloc(0)
    const fail = (err: Error): void => {
      cleanup()
      socket.destroy()
      reject(err)
    }
    const onData = (chunk: Buffer): void => {
      buffered = Buffer.concat([buffered, chunk])
      const newline = buffered.indexOf(0x0a)
      if (newline === -1) {
        if (buffered.length > 256) fail(peerError(name))
        return
      }
      const line = buffered.subarray(0, newline).toString('utf-8')
      const rest = buffered.subarray(newline + 1)
      // Paused until the caller has attached its own listeners, so no reply byte is emitted to nobody.
      socket.pause()
      cleanup()
      readFile(join(keyDir, `${name}.ipckey`), 'utf-8').then(
        (token) => {
          if (!line.startsWith(GREETING) || line.slice(GREETING.length) !== token.trim()) {
            socket.destroy()
            reject(peerError(name))
            return
          }
          if (rest.length > 0) socket.unshift(rest)
          resolve(socket)
          setImmediate(() => socket.resume())
        },
        (err) => {
          socket.destroy()
          reject(err)
        }
      )
    }
    const onError = (err: Error): void => fail(err)
    const onClose = (): void => fail(new Error(`ipc: ${name} closed before it greeted`))
    const timer = setTimeout(
      () => fail(new Error(`ipc: ${name} did not greet`)),
      GREETING_TIMEOUT_MS
    )
    function cleanup(): void {
      clearTimeout(timer)
      socket.removeListener('data', onData)
      socket.removeListener('error', onError)
      socket.removeListener('close', onClose)
    }
    socket.on('data', onData)
    socket.once('error', onError)
    socket.once('close', onClose)
  })
}

function peerError(name: IpcName): Error {
  return Object.assign(new Error(`ipc: the ${name} pipe is not held by lemongrass`), {
    code: 'EIPCPEER'
  })
}

// Resolves with a connected socket to a Go daemon, verified on Windows. Connection errors keep their code
// (ENOENT, ECONNREFUSED), which the callers' retry and probe logic reads.
export async function ipcConnect(name: Exclude<IpcName, 'app'>): Promise<net.Socket> {
  const address = await ipcAddress(name)
  const socket = await new Promise<net.Socket>((resolve, reject) => {
    const s = net.createConnection(address)
    s.once('connect', () => {
      s.removeListener('error', reject)
      resolve(s)
    })
    s.once('error', reject)
  })
  if (process.platform !== 'win32') return socket
  return verifyGreeting(socket, name)
}
