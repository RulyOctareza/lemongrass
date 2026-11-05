// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app } from 'electron'
import { spawn } from 'child_process'
import { existsSync } from 'fs'
import { dirname, join } from 'path'
import { winAssetName } from './updateVersion'
import { afterExitCommand, WIN_UNINSTALLER, winUnsupportedReason } from './updaterWinScript'
import type { UpdatePlatform } from './updatePlatform'

// Windows cannot replace the files of a running app, so installing only keeps the verified setup.exe, and restart
// runs it silently once the app has quit. --force-run makes the silent install start the new version.

let staged: string | null = null

function installDir(): string {
  return dirname(app.getPath('exe'))
}

async function install(
  file: string,
  version: string,
  onInstalling: () => void
): Promise<string | null> {
  onInstalling()
  staged = file
  return `Version ${version} is ready. Restart to install it. Restarting closes every open tab.`
}

function restart(): void {
  if (!staged) return
  const command = afterExitCommand(process.env.SystemRoot ?? 'C:\\Windows', process.pid, staged, [
    '/S',
    '--force-run'
  ])
  spawn(command.file, command.args, { detached: true, stdio: 'ignore', windowsHide: true }).unref()
  app.quit()
}

export const winPlatform: UpdatePlatform = {
  unsupportedReason: async () =>
    winUnsupportedReason(process.arch, existsSync(join(installDir(), WIN_UNINSTALLER))),
  assetName: winAssetName,
  installingMessage: 'Preparing the new version.',
  install,
  // The setup.exe runs only at restart, so it stays in its version's folder until the new version starts.
  keepAfterInstall: true,
  restart
}
