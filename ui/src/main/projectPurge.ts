// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile } from 'child_process'
import { loadProjects, saveProjects } from './projects'
import { dropNodeProject } from './node'
import { dropWorkspaceLayout } from './workspaceLayouts'
import { expiredProjectIds } from './projectPurgeCore'

const PURGE_EVERY_MS = 24 * 60 * 60 * 1000
const LGRASSD_TIMEOUT_MS = 30_000

function purgeGoData(lgrassdPath: string, id: string): Promise<void> {
  return new Promise((resolve, reject) => {
    execFile(
      lgrassdPath,
      ['project', 'purge', id],
      { encoding: 'utf8', timeout: LGRASSD_TIMEOUT_MS },
      (failure, _stdout, stderr) =>
        failure ? reject(new Error(stderr.trim() || failure.message)) : resolve()
    )
  })
}

// The registry entry goes last, so a purge that fails partway leaves the project marked deleted and the next run
// retries it. Removing the entry also lets the gatekeeper drop the project's policy.
async function purgeOne(lgrassdPath: string, id: string): Promise<void> {
  await purgeGoData(lgrassdPath, id)
  dropWorkspaceLayout(id)
  await dropNodeProject(id)
  saveProjects(loadProjects().filter((p) => p.id !== id))
}

async function purgeExpired(lgrassdPath: string): Promise<void> {
  for (const id of expiredProjectIds(loadProjects(), Date.now())) {
    try {
      await purgeOne(lgrassdPath, id)
    } catch (err) {
      console.error(`project purge ${id} failed:`, err)
    }
  }
}

// Runs only while the app is open; a project past its 30 days while the app is closed is purged at the next start.
export function startProjectPurge(lgrassdPath: string | null): void {
  if (!lgrassdPath) return
  void purgeExpired(lgrassdPath)
  setInterval(() => void purgeExpired(lgrassdPath), PURGE_EVERY_MS).unref()
}
