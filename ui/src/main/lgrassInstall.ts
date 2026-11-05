// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app } from 'electron'
import {
  existsSync,
  copyFileSync,
  chmodSync,
  mkdirSync,
  readdirSync,
  renameSync,
  unlinkSync
} from 'fs'
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
    case 'win32':
      osName = 'windows'
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
  // Linux and Windows packages carry only the x64 binaries; macOS carries both arches. Windows on arm64 runs the
  // x64 app under emulation, which reports x64 here.
  if (osName !== 'darwin' && arch !== 'amd64') return null
  return `${osName}-${arch}`
}

type BinName = 'lgrass' | 'lgrassd' | 'lgrassconf'

// The file name of a lemongrass binary on this OS, matching config.Exe in Go.
function exe(name: BinName): string {
  return process.platform === 'win32' ? `${name}.exe` : name
}

export function resolveBundledBin(name: BinName): string | null {
  const dir = resourceDir()
  if (!dir) return null
  return join(process.resourcesPath, 'bin', dir, exe(name))
}

// Must match config.BinDirs in Go, which `lgrassconf uninstall` cleans. On Windows the folder is always
// ~/.local/bin, which `lgrassconf install` puts on the user's PATH.
function resolveInstallDir(): string {
  const localBin = join(homedir(), '.local', 'bin')
  if (process.platform === 'win32') return localBin
  return existsSync(localBin) ? localBin : '/usr/local/bin'
}

// lgrassd lives under ~/.lemongrass/bin, off PATH, so a model cannot call it by name.
function lgrassdInstallDir(): string {
  return join(homedir(), '.lemongrass', 'bin')
}

function tryUnlink(path: string): void {
  try {
    unlinkSync(path)
  } catch {
    // ignore
  }
}

// Windows cannot overwrite a running exe but can rename it, so a copy in use is moved aside to <file>.old-* and the
// new one renamed into its place. The process running the old file keeps it open, so it is deleted on a later
// launch, once nothing runs it.
function removeSetAside(installDir: string, file: string): void {
  let entries: string[]
  try {
    entries = readdirSync(installDir)
  } catch {
    return
  }
  for (const entry of entries) {
    if (entry.startsWith(`${file}.old-`)) tryUnlink(join(installDir, entry))
  }
}

function replaceFile(tmp: string, dest: string): void {
  if (process.platform !== 'win32' || !existsSync(dest)) {
    renameSync(tmp, dest)
    return
  }
  try {
    renameSync(tmp, dest)
    return
  } catch {
    // dest is running; it is set aside below.
  }
  const aside = `${dest}.old-${process.pid}-${Date.now()}`
  renameSync(dest, aside)
  try {
    renameSync(tmp, dest)
  } catch (err) {
    renameSync(aside, dest)
    throw err
  }
  tryUnlink(aside)
}

function copyBundled(name: BinName, installDir: string = resolveInstallDir()): string | null {
  try {
    const bundled = resolveBundledBin(name)
    if (!bundled || !existsSync(bundled)) {
      console.error(`${name}: no bundled binary for this platform/arch, skipping self-install`)
      return null
    }
    mkdirSync(installDir, { recursive: true })
    removeSetAside(installDir, exe(name))
    const dest = join(installDir, exe(name))
    // dest may be the running lgrassconf service binary, which cannot be overwritten in place while open.
    const tmp = `${dest}.tmp-${process.pid}`
    copyFileSync(bundled, tmp)
    chmodSync(tmp, 0o755)
    try {
      replaceFile(tmp, dest)
    } catch (err) {
      tryUnlink(tmp)
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
    const dest = join(resolveInstallDir(), exe('lgrass'))
    if (!existsSync(dest)) {
      console.error('lgrass: not found at', dest, '. Run `make install` from the repo root.')
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
    const dest = join(lgrassdInstallDir(), exe('lgrassd'))
    if (!existsSync(dest)) {
      console.error('lgrassd: not found at', dest, '. Run `make install` from the repo root.')
      return null
    }
    return dest
  }
  return copyBundled('lgrassd', lgrassdInstallDir())
}

// Same pattern for lgrassconf. On packaged launches, also runs
// `lgrassconf install` so the agent config keeper is registered
// (LaunchAgent on macOS, systemd user unit on Linux, a logon task on
// Windows). Failures are logged, never fatal.
export function installLgrassconf(): string | null {
  if (!app.isPackaged) {
    const dest = join(resolveInstallDir(), exe('lgrassconf'))
    return existsSync(dest) ? dest : null
  }
  const dest = copyBundled('lgrassconf')
  if (!dest) return null
  try {
    const child = spawn(dest, ['install'], {
      detached: true,
      stdio: 'ignore',
      windowsHide: true
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
