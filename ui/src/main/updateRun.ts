// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { execFile } from 'child_process'
import { readFile } from 'fs/promises'
import { basename } from 'path'

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

// Like run, and calls onExec once the process replaces itself with another program, seen as a change of
// its /proc comm. pkexec execs its target in the same process once the password is accepted, so this
// is the only outside sign that the prompt is over. Linux only.
export function runUntilExec(
  command: string,
  args: string[],
  onExec: () => void,
  intervalMs = 200
): Promise<void> {
  return new Promise((resolve, reject) => {
    let exited = false
    let timer: NodeJS.Timeout | null = null
    const child = execFile(command, args, { encoding: 'utf8' }, (failure, _stdout, stderr) => {
      exited = true
      if (timer) clearTimeout(timer)
      if (!failure) return resolve()
      reject(Object.assign(new Error(stderr.trim() || failure.message), { code: failure.code }))
    })
    const pid = child.pid
    if (pid === undefined) return
    // Compared against the started program's name, not a first read, so an exec that happens before
    // the first poll, as when polkit still holds a recent authorization, is seen too. The kernel cuts
    // comm to 15 bytes.
    const started = basename(command).slice(0, 15)
    const poll = async (): Promise<void> => {
      if (exited) return
      let comm: string
      try {
        comm = (await readFile(`/proc/${pid}/comm`, 'utf8')).trim()
      } catch {
        return
      }
      if (exited) return
      if (comm !== started) {
        onExec()
        return
      }
      timer = setTimeout(() => void poll(), intervalMs)
    }
    void poll()
  })
}
