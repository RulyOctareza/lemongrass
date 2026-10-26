// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { app } from 'electron'
import { existsSync, copyFileSync, chmodSync, mkdirSync, renameSync, unlinkSync } from 'fs'
import { join } from 'path'
import { homedir } from 'os'
import { spawn } from 'child_process'

// Maps Node's process.platform + process.arch to the bundled resource
// subdirectory (Go GOOS-GOARCH naming from the root Makefile).
function resourceDir(): string | null {
  let osName: string
  switch (process.platform) {
    case 'linux':
      osName = 'linux'
      break
    case 'darwin':
      osName = 'darwin'
      break
    default:
      return null
  }
  let arch: string
  switch (process.arch) {
    case 'x64':
      arch = 'amd64'
      break
    case 'arm64':
      arch = 'arm64'
      break
    default:
      return null
  }
  return `${osName}-${arch}`
}

type BinName = 'lgrass' | 'lgrassd' | 'lgrassconf'

function resolveBundledBin(name: BinName): string | null {
  const dir = resourceDir()
  if (!dir) return null
  return join(process.resourcesPath, 'bin', dir, name)
}

function resolveInstallDir(): string {
  const localBin = join(homedir(), '.local', 'bin')
  return existsSync(localBin) ? localBin : '/usr/local/bin'
}

// lgrassd lives under ~/.lemongrass/bin, off PATH, so a model cannot call it by name.
function lgrassdInstallDir(): string {
  return join(homedir(), '.lemongrass', 'bin')
}

function copyBundled(name: BinName, installDir: string = resolveInstallDir()): string | null {
  try {
    const bundled = resolveBundledBin(name)
    if (!bundled || !existsSync(bundled)) {
      console.error(`${name}: no bundled binary for this platform/arch, skipping self-install`)
      return null
    }
    mkdirSync(installDir, { recursive: true })
    const dest = join(installDir, name)
    // dest may be the running lgrassconf service binary, which cannot be overwritten in place while open.
    const tmp = `${dest}.tmp-${process.pid}`
    copyFileSync(bundled, tmp)
    chmodSync(tmp, 0o755)
    try {
      renameSync(tmp, dest)
    } catch (err) {
      try {
        unlinkSync(tmp)
      } catch {
        // ignore
      }
      throw err
    }
    return dest
  } catch (err) {
    console.error(`${name}: self-install failed, continuing without it:`, err)
    return null
  }
}

// Copies the bundled lgrass binary matching this machine into
// ~/.local/bin (or /usr/local/bin as a fallback), keeping it in
// permanent version lockstep with the running app. Never throws.
//
// Unpackaged (electron-vite dev) has no resourcesPath bundle to copy
// from. `make install` builds lgrass straight to the install dir instead.
export function installLgrass(): string | null {
  if (!app.isPackaged) {
    const dest = join(resolveInstallDir(), 'lgrass')
    if (!existsSync(dest)) {
      console.error(
        'lgrass: not found at',
        dest,
        '. Run `make install` from the repo root.'
      )
      return null
    }
    return dest
  }
  return copyBundled('lgrass')
}

// Same pattern for lgrassd, installed into ~/.lemongrass/bin. Unpackaged,
// `make install` builds it there instead.
export function installLgrassd(): string | null {
  if (!app.isPackaged) {
    const dest = join(lgrassdInstallDir(), 'lgrassd')
    if (!existsSync(dest)) {
      console.error(
        'lgrassd: not found at',
        dest,
        '. Run `make install` from the repo root.'
      )
      return null
    }
    return dest
  }
  return copyBundled('lgrassd', lgrassdInstallDir())
}

// Same pattern for lgrassconf. On packaged launches, also runs
// `lgrassconf install` so the agent config keeper is registered
// (LaunchAgent on macOS, systemd user unit on Linux). Failures are
// logged, never fatal.
export function installLgrassconf(): string | null {
  if (!app.isPackaged) {
    const dest = join(resolveInstallDir(), 'lgrassconf')
    return existsSync(dest) ? dest : null
  }
  const dest = copyBundled('lgrassconf')
  if (!dest) return null
  try {
    const child = spawn(dest, ['install'], {
      detached: true,
      stdio: 'ignore'
    })
    child.on('error', (err) => {
      console.error('lgrassconf: install spawn failed:', err)
    })
    child.unref()
  } catch (err) {
    console.error('lgrassconf: install spawn failed:', err)
  }
  return dest
}
