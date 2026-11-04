// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync, spawnSync } from 'node:child_process'
import {
  chmodSync,
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  rmSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import {
  bundleFromExe,
  elevateScript,
  macUnsupportedReason,
  runnerScript,
  shellQuote,
  swapScript
} from './updateMacScripts.ts'

test('finds the bundle from the executable path', () => {
  assert.equal(
    bundleFromExe('/Applications/Lemongrass.app/Contents/MacOS/Lemongrass'),
    '/Applications/Lemongrass.app'
  )
  assert.equal(
    bundleFromExe('/Users/a b/Apps/Lemongrass.app/Contents/MacOS/Lemongrass'),
    '/Users/a b/Apps/Lemongrass.app'
  )
  assert.equal(bundleFromExe('/opt/Lemongrass/lemongrass'), null)
  assert.equal(bundleFromExe('/Applications/Lemongrass/Contents/MacOS/Lemongrass'), null)
})

test('refuses bundles it cannot replace', () => {
  assert.equal(macUnsupportedReason('/Applications/Lemongrass.app', 'arm64'), null)
  assert.equal(macUnsupportedReason('/Users/a/Applications/Lemongrass.app', 'x64'), null)
  assert.match(
    macUnsupportedReason('/Volumes/Lemongrass 0.4.0/Lemongrass.app', 'arm64') ?? '',
    /disk image/
  )
  assert.match(
    macUnsupportedReason('/private/var/folders/x/AppTranslocation/ABC/d/Lemongrass.app', 'arm64') ??
      '',
    /temporary location/
  )
  assert.match(macUnsupportedReason(null, 'arm64') ?? '', /bundle/)
  assert.match(macUnsupportedReason('/Applications/Lemongrass.app', 'ia32') ?? '', /ia32/)
})

test('quotes shell values', () => {
  assert.equal(shellQuote("it's here"), `'it'\\''s here'`)
  assert.equal(
    execFileSync('bash', ['-c', `printf %s ${shellQuote("a 'b' $HOME")}`], { encoding: 'utf8' }),
    "a 'b' $HOME"
  )
})

test('escapes the swap path inside the AppleScript string', () => {
  assert.equal(
    elevateScript('/Users/a "b"\\c/swap.sh').split(' & ')[1].split(' with prompt')[0],
    'quoted form of "/Users/a \\"b\\"\\\\c/swap.sh"'
  )
})

// ditto, xattr and open are macOS tools, so stand-ins on PATH let the scripts run anywhere.
const roots: string[] = []
after(() => roots.forEach((root) => rmSync(root, { recursive: true, force: true })))

function sandbox(): { root: string; env: NodeJS.ProcessEnv } {
  const root = mkdtempSync(join(tmpdir(), "lg update 'test' "))
  roots.push(root)
  const bin = join(root, 'bin')
  mkdirSync(bin)
  const stub = (name: string, body: string): void => {
    writeFileSync(join(bin, name), `#!/bin/bash\n${body}\n`)
    chmodSync(join(bin, name), 0o755)
  }
  stub('ditto', '[ -n "${FAIL_DITTO:-}" ] && exit 1\ncp -R "$1" "$2"')
  stub('xattr', 'exit 0')
  stub('open', `echo "$1" >> ${shellQuote(join(root, 'opened'))}`)
  return { root, env: { ...process.env, PATH: `${bin}:${process.env.PATH}` } }
}

function app(path: string, version: string): void {
  mkdirSync(join(path, 'Contents'), { recursive: true })
  writeFileSync(join(path, 'Contents', 'version'), version)
}

test('the swap replaces the installed app', () => {
  const { root, env } = sandbox()
  const bundle = join(root, 'Apps', 'Lemongrass.app')
  const staged = join(root, 'staged', 'Lemongrass.app')
  app(bundle, 'old')
  app(staged, 'new')
  writeFileSync(join(root, 'swap.sh'), swapScript(bundle, staged))
  const result = spawnSync('bash', [join(root, 'swap.sh')], { env, encoding: 'utf8' })
  assert.equal(result.status, 0, result.stdout + result.stderr)
  assert.equal(readFileSync(join(bundle, 'Contents', 'version'), 'utf8'), 'new')
  assert.equal(existsSync(join(root, 'Apps', '.Lemongrass-old.app')), false)
  assert.equal(existsSync(join(root, 'Apps', '.Lemongrass-update.app')), false)
})

test('a failed copy leaves the installed app in place', () => {
  const { root, env } = sandbox()
  const bundle = join(root, 'Apps', 'Lemongrass.app')
  const staged = join(root, 'staged', 'Lemongrass.app')
  app(bundle, 'old')
  app(staged, 'new')
  writeFileSync(join(root, 'swap.sh'), swapScript(bundle, staged))
  const result = spawnSync('bash', [join(root, 'swap.sh')], {
    env: { ...env, FAIL_DITTO: '1' },
    encoding: 'utf8'
  })
  assert.equal(result.status, 1)
  assert.equal(readFileSync(join(bundle, 'Contents', 'version'), 'utf8'), 'old')
})

test('the runner waits for the app, swaps and opens it again', () => {
  const { root, env } = sandbox()
  const bundle = join(root, 'Apps', 'Lemongrass.app')
  const staged = join(root, 'staged', 'Lemongrass.app')
  app(bundle, 'old')
  app(staged, 'new')
  const exited = spawnSync('true')
  const paths = {
    pid: exited.pid,
    bundle,
    swap: join(root, 'swap.sh'),
    elevate: join(root, 'elevate.applescript'),
    log: join(root, 'update.log')
  }
  writeFileSync(paths.swap, swapScript(bundle, staged))
  writeFileSync(paths.elevate, elevateScript(paths.swap))
  writeFileSync(join(root, 'runner.sh'), runnerScript(paths))
  const result = spawnSync('bash', [join(root, 'runner.sh')], { env, encoding: 'utf8' })
  const log = readFileSync(paths.log, 'utf8')
  assert.equal(result.status, 0, log)
  assert.equal(readFileSync(join(bundle, 'Contents', 'version'), 'utf8'), 'new')
  assert.equal(readFileSync(join(root, 'opened'), 'utf8').trim(), bundle)
  assert.match(log, /swap: done/)
})
