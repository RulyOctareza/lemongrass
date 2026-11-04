// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { app } from 'electron'
import { rpmAssetName } from './updateVersion'
import { run } from './updateRun'
import type { UpdatePlatform } from './updatePlatform'

const RPM = '/usr/bin/rpm'

// Updating replaces the rpm's files under /opt, so only a copy that rpm itself installed can do it.
async function unsupportedReason(): Promise<string | null> {
  if (process.arch !== 'x64') return 'Updates are published for Linux x86_64 only.'
  try {
    await run(RPM, ['-q', 'lemongrass'])
  } catch {
    return 'This copy was not installed from the rpm, so it cannot update itself.'
  }
  return null
}

// pkexec shows the desktop's polkit password prompt. It exits 126 when the prompt is dismissed.
async function install(file: string): Promise<string | null> {
  try {
    await run('pkexec', [RPM, '-U', file])
  } catch (err) {
    if ((err as { code?: unknown }).code === 126) {
      throw new Error('updater: the password prompt was dismissed, nothing was installed')
    }
    throw err
  }
  return null
}

export const rpmPlatform: UpdatePlatform = {
  unsupportedReason,
  assetName: rpmAssetName,
  installingMessage: null,
  install,
  keepAfterInstall: false,
  restart(): void {
    app.relaunch()
    app.quit()
  }
}
