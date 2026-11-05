// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app } from 'electron'
import { spawn } from 'child_process'
import {
  accessSync,
  constants,
  existsSync,
  mkdirSync,
  readdirSync,
  rmSync,
  writeFileSync
} from 'fs'
import { dirname, join } from 'path'
import { dmgAssetName } from './updateVersion'
import { run } from './updateRun'
import {
  bundleFromExe,
  elevateScript,
  macUnsupportedReason,
  runnerScript,
  swapScript
} from './updateMacScripts'
import type { UpdatePlatform } from './updatePlatform'

// The app is signed ad hoc and not notarized, so Squirrel.Mac, which demands the same Developer ID on
// every version, cannot be used. A dmg the app downloads itself carries no com.apple.quarantine, so the
// replaced app passes Gatekeeper without the "could not verify" dialog; quarantine is cleared anyway in
// case a future macOS starts tagging it.

const bundle = bundleFromExe(app.getPath('exe'))
let staged: string | null = null

function writable(path: string): boolean {
  try {
    accessSync(path, constants.W_OK)
    return true
  } catch {
    return false
  }
}

function needsPassword(): boolean {
  return !bundle || !writable(dirname(bundle)) || !writable(bundle)
}

async function detach(mount: string): Promise<void> {
  try {
    await run('hdiutil', ['detach', '-quiet', mount])
  } catch {
    await run('hdiutil', ['detach', '-quiet', '-force', mount]).catch(() => undefined)
  }
}

// Copies the app out of the verified dmg into the version's download folder. The dmg stays mounted only
// for the copy.
async function install(
  file: string,
  version: string,
  onInstalling: () => void
): Promise<string | null> {
  onInstalling()
  const dir = dirname(file)
  const mount = join(dir, 'mnt')
  const stageDir = join(dir, 'app')
  if (existsSync(mount)) await detach(mount)
  rmSync(stageDir, { recursive: true, force: true })
  mkdirSync(mount, { recursive: true })
  mkdirSync(stageDir, { recursive: true })

  await run('hdiutil', [
    'attach',
    '-nobrowse',
    '-readonly',
    '-noautoopen',
    '-mountpoint',
    mount,
    file
  ])
  let copy: string
  try {
    const apps = readdirSync(mount).filter((entry) => entry.endsWith('.app'))
    if (apps.length !== 1)
      throw new Error(`updater: the dmg holds ${apps.length} apps, expected one`)
    copy = join(stageDir, apps[0])
    await run('ditto', [join(mount, apps[0]), copy])
  } finally {
    await detach(mount)
  }
  await run('xattr', ['-dr', 'com.apple.quarantine', copy]).catch(() => undefined)
  await run('codesign', ['--verify', '--deep', '--strict', copy])
  staged = copy

  const password = needsPassword() ? ' Restarting asks for your password.' : ''
  return `Version ${version} is ready. Restart to use it. Restarting closes every open tab.${password}`
}

function restart(): void {
  if (!staged || !bundle) return
  const dir = dirname(dirname(staged))
  const swap = join(dir, 'swap.sh')
  const elevate = join(dir, 'elevate.applescript')
  const runner = join(dir, 'runner.sh')
  writeFileSync(swap, swapScript(bundle, staged))
  writeFileSync(elevate, elevateScript(swap))
  writeFileSync(
    runner,
    runnerScript({ pid: process.pid, bundle, swap, elevate, log: join(dir, 'update.log') })
  )
  // Detached into its own process group, so it outlives the app it waits for.
  spawn('/bin/bash', [runner], { detached: true, stdio: 'ignore' }).unref()
  app.quit()
}

export const macPlatform: UpdatePlatform = {
  unsupportedReason: async () => macUnsupportedReason(bundle, process.arch),
  assetName: (version) => dmgAssetName(version, process.arch),
  installingMessage: 'Preparing the new version.',
  install,
  keepAfterInstall: true,
  restart
}
