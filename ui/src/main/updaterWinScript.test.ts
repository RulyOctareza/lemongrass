// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { afterExitCommand, winUnsupportedReason } from './updaterWinScript.ts'

test('afterExitCommand waits for the pid and quotes the program for PowerShell', () => {
  const { file, args } = afterExitCommand(
    'C:\\Windows',
    4242,
    "C:\\Users\\O'Neil\\.lemongrass\\updates\\1.0.0\\lemongrass-1.0.0-setup.exe",
    ['/S', '--force-run']
  )
  assert.equal(file, 'C:\\Windows\\System32\\WindowsPowerShell\\v1.0\\powershell.exe')
  assert.deepEqual(args.slice(0, 5), [
    '-NoProfile',
    '-NonInteractive',
    '-WindowStyle',
    'Hidden',
    '-Command'
  ])
  assert.equal(
    args[5],
    "Wait-Process -Id 4242 -Timeout 120 -ErrorAction SilentlyContinue; Start-Process -FilePath 'C:\\Users\\O''Neil\\.lemongrass\\updates\\1.0.0\\lemongrass-1.0.0-setup.exe' -ArgumentList '/S','--force-run'"
  )
})

test('winUnsupportedReason needs x64 and an installed copy', () => {
  assert.equal(winUnsupportedReason('x64', true), null)
  assert.match(winUnsupportedReason('arm64', true) ?? '', /x64 only/)
  assert.match(winUnsupportedReason('x64', false) ?? '', /not installed from the setup/)
})
