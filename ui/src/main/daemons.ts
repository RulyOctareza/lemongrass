// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { spawn, ChildProcess } from 'child_process'
import { mkdirSync, openSync } from 'fs'
import { homedir } from 'os'
import { join } from 'path'
import { ipcConnect } from './ipcAddress'

// True if our daemon already listens under name -- one started manually, or
// left running by a previous launch. A short timeout stands in for the
// ENOENT/ECONNREFUSED case (nothing there yet). On Windows a pipe that does
// not greet with our token is not ours: the daemon spawned next fails to take
// the name and logs it.
function probe(name: 'vault' | 'agent' | 'node'): Promise<boolean> {
  return new Promise((resolve) => {
    const timer = setTimeout(() => resolve(false), 300)
    ipcConnect(name).then(
      (socket) => {
        clearTimeout(timer)
        socket.destroy()
        resolve(true)
      },
      (err) => {
        clearTimeout(timer)
        if ((err as NodeJS.ErrnoException)?.code === 'EIPCPEER')
          console.error(`lgrassd ${name}:`, err)
        resolve(false)
      }
    )
  })
}

const spawned: ChildProcess[] = []

function spawnDaemon(lgrassdPath: string, subcommand: 'vault' | 'agent' | 'node'): void {
  const logDir = join(homedir(), '.lemongrass', 'logs')
  mkdirSync(logDir, { recursive: true })
  const logFd = openSync(join(logDir, `${subcommand}.log`), 'a')
  const proc = spawn(lgrassdPath, [subcommand, 'run'], {
    stdio: ['ignore', logFd, logFd],
    windowsHide: true
  })
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
  if (!(await probe('vault'))) spawnDaemon(lgrassdPath, 'vault')
  if (!(await probe('agent'))) spawnDaemon(lgrassdPath, 'agent')
  if (!(await probe('node'))) spawnDaemon(lgrassdPath, 'node')
}

// Kills only the daemons this session itself spawned -- one that was
// already running before launch (started manually, or by another instance)
// is left alone.
export function killDaemons(): void {
  for (const proc of spawned) proc.kill()
  spawned.length = 0
}
