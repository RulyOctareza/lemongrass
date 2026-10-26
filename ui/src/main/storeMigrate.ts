// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { dialog } from 'electron'
import { execFile } from 'child_process'

// Longer than the tab calls' timeout, since a migration can rebuild a table.
const MIGRATE_TIMEOUT_MS = 30_000

// Never rejects. A failure is shown rather than ignored, since every tab depends on the store, and the boot goes on: each store open still migrates as the fallback.
export function migrateStore(lgrassdPath: string): Promise<void> {
  return new Promise((resolve) => {
    execFile(
      lgrassdPath,
      ['migrate'],
      { encoding: 'utf8', timeout: MIGRATE_TIMEOUT_MS },
      (failure, _stdout, stderr) => {
        if (failure) {
          const detail = stderr.trim() || failure.message
          console.error('lgrassd migrate failed:', detail)
          dialog.showErrorBox(
            'Lemongrass could not update its store',
            `Tabs may not receive notifications until this is fixed.\n\n${detail}`
          )
        }
        resolve()
      }
    )
  })
}
