// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile } from 'child_process'

// Rejects with stderr as the message and the exit code on the error, so a caller can tell a dismissed
// password prompt from a failed install.
export function run(command: string, args: string[]): Promise<void> {
  return new Promise((resolve, reject) => {
    execFile(command, args, { encoding: 'utf8' }, (failure, _stdout, stderr) => {
      if (!failure) return resolve()
      reject(Object.assign(new Error(stderr.trim() || failure.message), { code: failure.code }))
    })
  })
}
