// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { spawn, ChildProcess } from 'child_process'
import { mkdirSync, openSync } from 'fs'
import * as net from 'net'
import { homedir } from 'os'
import { join } from 'path'

function vaultSocketPath(): string {
  return join(homedir(), '.lemongrass', 'vault.sock')
}

function agentSocketPath(): string {
  return join(homedir(), '.lemongrass', 'agent.sock')
}

export function nodeSocketPath(): string {
  return join(homedir(), '.lemongrass', 'node.sock')
}

// True if something is already listening on sockPath -- a daemon started
// manually, or left running by a previous launch. A short timeout stands in
// for the ENOENT/ECONNREFUSED case (nothing there yet).
function probe(sockPath: string): Promise<boolean> {
  return new Promise((resolve) => {
    const socket = net.createConnection(sockPath)
    const finish = (ok: boolean): void => {
      socket.removeAllListeners()
      socket.destroy()
      resolve(ok)
    }
    socket.setTimeout(300, () => finish(false))
    socket.once('connect', () => finish(true))
    socket.once('error', () => finish(false))
  })
}

const spawned: ChildProcess[] = []

function spawnDaemon(lgrassdPath: string, subcommand: 'vault' | 'agent' | 'node'): void {
  const logDir = join(homedir(), '.lemongrass', 'logs')
  mkdirSync(logDir, { recursive: true })
  const logFd = openSync(join(logDir, `${subcommand}.log`), 'a')
  const proc = spawn(lgrassdPath, [subcommand, 'run'], { stdio: ['ignore', logFd, logFd] })
  proc.on('error', (err) => {
    console.error(`lgrassd ${subcommand}: failed to start:`, err)
  })
  spawned.push(proc)
}

// Starts the vault, agent and node daemons if nothing is listening on their
// sockets yet. Never throws -- a launch never fails over this, matching
// installLgrassd's own "never fails a launch" stance; a failure to start
// just leaves the sockets missing, which the vault IPC calls already
// surface as their own error.
export async function ensureDaemonsRunning(lgrassdPath: string): Promise<void> {
  if (!(await probe(vaultSocketPath()))) spawnDaemon(lgrassdPath, 'vault')
  if (!(await probe(agentSocketPath()))) spawnDaemon(lgrassdPath, 'agent')
  if (!(await probe(nodeSocketPath()))) spawnDaemon(lgrassdPath, 'node')
}

// Kills only the daemons this session itself spawned -- one that was
// already running before launch (started manually, or by another instance)
// is left alone.
export function killDaemons(): void {
  for (const proc of spawned) proc.kill()
  spawned.length = 0
}
