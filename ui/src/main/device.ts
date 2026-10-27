// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { ipcMain, type WebContents } from 'electron'
import { execFile, spawn, type ChildProcess } from 'child_process'
import { accessSync, constants, realpathSync } from 'fs'
import { createServer, connect, type Socket } from 'net'
import { homedir } from 'os'
import { delimiter, dirname, join } from 'path'
import { ScrcpyVideoParser } from './scrcpyStream'
import type {
  DeviceEnd,
  DeviceInfo,
  DeviceListResult,
  DevicePacket,
  DeviceStartRequest,
  DeviceStartResult
} from '../preload/types'

const CONNECT_ATTEMPTS = 100
const CONNECT_RETRY_MS = 100
const ADB_TIMEOUT_MS = 15_000
// The longest side of the mirrored video; the phone's own resolution is kept when smaller.
const MAX_SIZE = 1600
const BIT_RATE = 8_000_000

interface Session {
  serial: string
  port?: number
  server?: ChildProcess
  video?: Socket
  control?: Socket
  ended: boolean
}

const sessions = new Map<string, Session>()

// PATH already holds the login shell's entries (main/loginPath.ts); the Android SDK's
// platform-tools come first because Android Studio installs adb there without touching PATH.
function toolDirs(): string[] {
  const sdk = [
    process.env.ANDROID_HOME,
    process.env.ANDROID_SDK_ROOT,
    join(homedir(), 'Library', 'Android', 'sdk')
  ]
    .filter((dir): dir is string => !!dir)
    .map((dir) => join(dir, 'platform-tools'))
  return [...sdk, ...(process.env.PATH ?? '').split(delimiter)].filter((dir) => dir.length > 0)
}

function findTool(name: string): string | null {
  for (const dir of toolDirs()) {
    const candidate = join(dir, name)
    try {
      accessSync(candidate, constants.X_OK)
      return candidate
    } catch {
      // Not installed in this directory.
    }
  }
  return null
}

function run(file: string, args: string[]): Promise<{ ok: boolean; out: string }> {
  return new Promise((resolve) => {
    execFile(file, args, { timeout: ADB_TIMEOUT_MS, encoding: 'utf8' }, (failure, stdout, stderr) =>
      resolve({ ok: !failure, out: failure ? stderr || failure.message : stdout })
    )
  })
}

interface ScrcpyServer {
  path: string
  version: string
}

// The server must be started with the exact version of the scrcpy install it ships with,
// so both come from the installed scrcpy rather than a bundled copy.
async function findScrcpyServer(): Promise<ScrcpyServer | string> {
  const scrcpy = findTool('scrcpy')
  if (!scrcpy) return 'scrcpy is not installed. Install it with "brew install scrcpy".'
  const version = /^scrcpy (\S+)/m.exec((await run(scrcpy, ['--version'])).out)?.[1]
  if (!version) return 'Could not read the scrcpy version.'
  const candidates = [
    process.env.SCRCPY_SERVER_PATH,
    join(dirname(dirname(realpathSync(scrcpy))), 'share', 'scrcpy', 'scrcpy-server'),
    '/usr/local/share/scrcpy/scrcpy-server',
    '/usr/share/scrcpy/scrcpy-server'
  ].filter((path): path is string => !!path)
  for (const path of candidates) {
    try {
      accessSync(path, constants.R_OK)
      return { path, version }
    } catch {
      // Try the next install layout.
    }
  }
  return 'The scrcpy-server file of the installed scrcpy was not found.'
}

export function parseAdbDevices(output: string): DeviceInfo[] {
  return output
    .split('\n')
    .slice(1)
    .map((line) => line.trim().split(/\s+/))
    .filter((fields) => fields.length >= 2 && fields[0].length > 0)
    .map(([serial, state, ...rest]) => {
      const model = rest.find((field) => field.startsWith('model:'))
      return {
        serial,
        state,
        model: model ? model.slice('model:'.length).replace(/_/g, ' ') : serial
      }
    })
}

async function listDevices(): Promise<DeviceListResult> {
  const adb = findTool('adb')
  if (!adb) return { ok: false, error: 'adb is not installed. Install Android platform-tools.' }
  const result = await run(adb, ['devices', '-l'])
  if (!result.ok) return { ok: false, error: result.out.trim() }
  return { ok: true, devices: parseAdbDevices(result.out) }
}

function freePort(): Promise<number> {
  return new Promise((resolve, reject) => {
    const server = createServer()
    server.once('error', reject)
    server.listen(0, '127.0.0.1', () => {
      const address = server.address()
      server.close(() => resolve(typeof address === 'object' && address ? address.port : 0))
    })
  })
}

// adb accepts a forwarded connection before the server listens and closes it right away,
// so a socket counts as connected only once the server's first byte arrives. The socket is
// handed back paused, with the bytes read so far.
function connectWhenReady(port: number, session: Session): Promise<[Socket, Buffer]> {
  return new Promise((resolve, reject) => {
    let attempt = 0
    const tryOnce = (): void => {
      if (session.ended) return reject(new Error('The device session ended while connecting.'))
      const socket = connect(port, '127.0.0.1')
      socket.on('error', () => {})
      const onData = (chunk: Buffer): void => {
        socket.off('data', onData)
        socket.pause()
        resolve([socket, chunk])
      }
      socket.on('data', onData)
      socket.once('close', () => {
        if (socket.bytesRead > 0) return
        if (++attempt >= CONNECT_ATTEMPTS) {
          return reject(new Error('The scrcpy server on the device did not answer.'))
        }
        setTimeout(tryOnce, CONNECT_RETRY_MS)
      })
    }
    tryOnce()
  })
}

function connectControl(port: number): Promise<Socket> {
  return new Promise((resolve, reject) => {
    const socket = connect(port, '127.0.0.1')
    socket.once('connect', () => resolve(socket))
    socket.once('error', reject)
  })
}

function stopSession(id: string, adb: string | null): void {
  const session = sessions.get(id)
  if (!session) return
  sessions.delete(id)
  session.ended = true
  session.video?.destroy()
  session.control?.destroy()
  session.server?.kill()
  if (adb && session.port) {
    void run(adb, ['-s', session.serial, 'forward', '--remove', `tcp:${session.port}`])
  }
}

export function stopAllDevices(): void {
  const adb = findTool('adb')
  for (const id of [...sessions.keys()]) stopSession(id, adb)
}

async function startSession(
  request: DeviceStartRequest,
  getContents: () => WebContents | undefined
): Promise<DeviceStartResult> {
  const adb = findTool('adb')
  if (!adb) return { ok: false, error: 'adb is not installed. Install Android platform-tools.' }

  const { id, serial } = request
  // Registered before the first await, so a stop that arrives while adb is still pushing or
  // forwarding cancels this start instead of leaving an unowned server on the phone.
  const session: Session = { serial, ended: false }
  sessions.set(id, session)
  // One mirror per phone: a second Device tab or a remounted pane takes the phone over.
  for (const [other, existing] of sessions) {
    if (other === id || existing.serial !== serial) continue
    stopSession(other, adb)
    const replaced: DeviceEnd = { id: other, replaced: true }
    getContents()?.send('device:end', replaced)
  }
  const stopped: DeviceStartResult = { ok: false, error: 'The device session was stopped.' }
  const failed = (error: string): DeviceStartResult => {
    stopSession(id, adb)
    return { ok: false, error }
  }

  const server = await findScrcpyServer()
  if (session.ended) return stopped
  if (typeof server === 'string') return failed(server)

  const remote = `/data/local/tmp/lemongrass-scrcpy-${server.version}.jar`
  const pushed = await run(adb, ['-s', serial, 'push', server.path, remote])
  if (session.ended) return stopped
  if (!pushed.ok) return failed(pushed.out.trim())

  // A random scid keeps this session's socket apart from any other scrcpy running on the phone.
  const scid = Math.floor(Math.random() * 0x7fffffff)
    .toString(16)
    .padStart(8, '0')
  const port = await freePort()
  if (session.ended) return stopped
  const forwarded = await run(adb, [
    '-s',
    serial,
    'forward',
    `tcp:${port}`,
    `localabstract:scrcpy_${scid}`
  ])
  if (session.ended) {
    if (forwarded.ok) void run(adb, ['-s', serial, 'forward', '--remove', `tcp:${port}`])
    return stopped
  }
  if (!forwarded.ok) return failed(forwarded.out.trim())
  session.port = port

  const child = spawn(adb, [
    '-s',
    serial,
    'shell',
    `CLASSPATH=${remote}`,
    'app_process',
    '/',
    'com.genymobile.scrcpy.Server',
    server.version,
    `scid=${scid}`,
    'log_level=warn',
    'tunnel_forward=true',
    'audio=false',
    'control=true',
    'clipboard_autosync=false',
    'video_codec=h264',
    `max_size=${MAX_SIZE}`,
    `video_bit_rate=${BIT_RATE}`
  ])
  session.server = child

  let serverOutput = ''
  child.stdout?.on('data', (chunk: Buffer) => (serverOutput += chunk.toString()))
  child.stderr?.on('data', (chunk: Buffer) => (serverOutput += chunk.toString()))
  const end = (error?: string): void => {
    if (!sessions.has(id)) return
    stopSession(id, adb)
    getContents()?.send('device:end', { id, ...(error ? { error } : {}) })
  }
  child.once('exit', () => end(serverOutput.trim() || undefined))

  try {
    const [video, first] = await connectWhenReady(port, session)
    session.video = video
    // The server waits for the control socket before it starts streaming.
    session.control = await connectControl(port)
    session.control.on('data', () => {})
    session.control.on('error', () => {})

    return await new Promise<DeviceStartResult>((resolve) => {
      const parser = new ScrcpyVideoParser({
        meta: (meta) => resolve({ ok: true, ...meta }),
        packet: (packet) => {
          const payload: DevicePacket = { id, ...packet }
          getContents()?.send('device:packet', payload)
        }
      })
      parser.push(first)
      video.on('data', (chunk: Buffer) => parser.push(chunk))
      video.once('close', () => {
        resolve({ ok: false, error: 'The device closed the video stream.' })
        end()
      })
      video.resume()
    })
  } catch (failure) {
    stopSession(id, adb)
    const detail = serverOutput.trim()
    return { ok: false, error: detail || (failure as Error).message }
  }
}

export function registerDeviceHandlers(getContents: () => WebContents | undefined): void {
  ipcMain.handle('device:list', () => listDevices())
  ipcMain.handle('device:start', (_event, request: DeviceStartRequest) =>
    startSession(request, getContents)
  )
  ipcMain.on('device:stop', (_event, id: string) => stopSession(id, findTool('adb')))
  // The renderer encodes control messages; main only forwards them to the session's socket.
  ipcMain.on('device:control', (_event, id: string, message: Uint8Array) => {
    const control = sessions.get(id)?.control
    if (control && !control.destroyed) control.write(Buffer.from(message))
  })
}
