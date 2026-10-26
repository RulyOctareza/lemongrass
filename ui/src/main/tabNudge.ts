// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { execFile } from 'child_process'
import { findShellByTab } from './pty'

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/
const LGRASS_TIMEOUT_MS = 5_000
// The line and its Enter go in separate writes with a pause between, for every vendor: an agent CLI that receives both in one write can keep the Enter as part of the input instead of submitting it.
const SUBMIT_DELAY_MS = 50

export interface NudgeReply {
  ok: boolean
  typed?: boolean
  reason?: string
  error?: string
}

function composeNudge(lgrassdPath: string, cwd: string, tabId: string): Promise<string> {
  return new Promise((resolve, reject) => {
    execFile(
      lgrassdPath,
      ['nudge', tabId],
      { cwd, encoding: 'utf8', timeout: LGRASS_TIMEOUT_MS },
      (failure, stdout) => (failure ? reject(failure) : resolve(stdout))
    )
  })
}

function printable(text: string): string {
  return [...text]
    .map((ch) => (ch.charCodeAt(0) < 32 || ch.charCodeAt(0) === 127 ? ' ' : ch))
    .join('')
    .trim()
}

// The request carries only a tab id, so the typed line always comes from lgrass's own ledger and no caller can type text of its own choosing.
export async function handleNudge(lgrassdPath: string | null, tabId: string): Promise<NudgeReply> {
  if (!lgrassdPath) return { ok: false, error: 'lgrassd is not installed' }
  if (!UUID.test(tabId)) return { ok: false, error: 'the tab id is not valid' }
  const shell = findShellByTab(tabId)
  if (!shell) return { ok: false, error: 'no open tab has that id' }
  if (shell.humanIsTyping()) return { ok: true, typed: false, reason: 'typing' }

  const text = printable(await composeNudge(lgrassdPath, shell.cwd, tabId))
  if (!text) return { ok: true, typed: false, reason: 'nothing pending' }
  await submit(shell, text)
  return { ok: true, typed: true }
}

async function submit(
  shell: NonNullable<ReturnType<typeof findShellByTab>>,
  text: string
): Promise<void> {
  shell.type(text)
  await new Promise((resolve) => setTimeout(resolve, SUBMIT_DELAY_MS))
  shell.type('\r')
}

const TYPING_WAIT_MS = 30_000
const TYPING_POLL_MS = 500

// Types a line composed by main itself into a tab, waiting while the human types there. Callers pass only text built from ids, never model-written text.
export async function typeSystemLine(tabId: string, text: string): Promise<boolean> {
  const deadline = Date.now() + TYPING_WAIT_MS
  for (;;) {
    const shell = findShellByTab(tabId)
    if (!shell) return false
    if (!shell.humanIsTyping()) {
      await submit(shell, printable(text))
      return true
    }
    if (Date.now() > deadline) return false
    await new Promise((resolve) => setTimeout(resolve, TYPING_POLL_MS))
  }
}
