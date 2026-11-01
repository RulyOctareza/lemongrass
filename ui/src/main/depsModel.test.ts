// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  KEPT_LINES,
  appendOutput,
  helperExitText,
  installUnsupported,
  nextSeen,
  offerBlocked,
  parseReport,
  requiredRegressed,
  successText
} from './depsModel.ts'

// The documented shape of `lgrassd deps check --json`, one entry per status, origin and offer case.
const fixture = {
  os: { family: 'fedora', id: 'fedora', version: '43' },
  deps: [
    {
      name: 'git',
      required: true,
      features: ['gitPane', 'conflicts', 'rebaseForecast', 'biblioHistory', 'workspaces'],
      floor: '2.45.0',
      path: '/usr/bin/git',
      version: '2.56.0',
      origin: { kind: 'package', package: 'git-core' },
      status: 'ok',
      offer: null,
      message: ''
    },
    {
      name: 'adb',
      required: false,
      features: ['device', 'flutterRecording'],
      floor: '',
      path: '',
      version: '',
      origin: { kind: 'none' },
      status: 'missing',
      offer: { action: 'install', package: 'android-tools', route: 'helper' },
      message: ''
    },
    {
      name: 'scrcpy',
      required: false,
      features: ['device'],
      floor: '',
      path: '',
      version: '',
      origin: { kind: 'none' },
      status: 'missing',
      offer: null,
      message: 'scrcpy is not in the enabled repositories.'
    },
    {
      name: 'ffmpeg',
      required: false,
      features: ['flutterRecording'],
      floor: '',
      path: '/usr/local/bin/ffmpeg',
      version: '',
      origin: { kind: 'manual' },
      status: 'ok',
      offer: null,
      message: ''
    },
    {
      name: 'ssh',
      required: false,
      features: ['sshChannels'],
      floor: '',
      path: '/usr/bin/ssh',
      version: '10.0',
      origin: { kind: 'unknown' },
      status: 'ok',
      offer: null,
      message: ''
    }
  ]
}

function withGit(patch: Record<string, unknown>): string {
  return JSON.stringify({
    ...fixture,
    deps: [{ ...fixture.deps[0], ...patch }, ...fixture.deps.slice(1)]
  })
}

test('parseReport reads every entry of the documented shape', () => {
  const report = parseReport(JSON.stringify(fixture))
  assert.ok(report)
  assert.equal(report.os.version, '43')
  assert.deepEqual(
    report.deps.map((dep) => [dep.name, dep.status, dep.origin.kind]),
    [
      ['git', 'ok', 'package'],
      ['adb', 'missing', 'none'],
      ['scrcpy', 'missing', 'none'],
      ['ffmpeg', 'ok', 'manual'],
      ['ssh', 'ok', 'unknown']
    ]
  )
  assert.deepEqual(report.deps[0].origin, { kind: 'package', package: 'git-core' })
  assert.deepEqual(report.deps[1].offer, {
    action: 'install',
    package: 'android-tools',
    route: 'helper'
  })
})

test('parseReport refuses the whole report when one entry is unreadable', () => {
  assert.equal(parseReport('not json'), null)
  assert.equal(parseReport('{"os":{}}'), null)
  assert.equal(parseReport(withGit({ status: 'fine' })), null)
  assert.equal(parseReport(withGit({ origin: { kind: 'package' } })), null)
  assert.equal(
    parseReport(withGit({ offer: { action: 'remove', package: 'git', route: 'helper' } })),
    null
  )
  assert.equal(
    parseReport(withGit({ offer: { action: 'upgrade', package: '', route: 'helper' } })),
    null
  )
})

test('installUnsupported names a source build and other systems', () => {
  assert.match(installUnsupported(false, 'linux', 'x64') ?? '', /source build/)
  assert.match(installUnsupported(true, 'win32', 'x64') ?? '', /not supported/)
  assert.equal(installUnsupported(true, 'linux', 'x64'), null)
  assert.equal(installUnsupported(true, 'linux', 'arm64'), null)
  assert.match(installUnsupported(true, 'linux', 'ia32') ?? '', /not supported/)
})

test('offerBlocked lets only the helper route run', () => {
  const offer = { action: 'install' as const, package: 'android-tools', route: 'helper' as const }
  assert.equal(offerBlocked(offer, null), null)
  assert.match(offerBlocked(offer, 'source build') ?? '', /source build/)
  assert.ok(offerBlocked({ ...offer, route: 'winget' }, null))
  assert.ok(offerBlocked(null, null))
})

test('appendOutput splits on carriage returns and keeps the newest lines', () => {
  let [lines, rest] = appendOutput([], '', 'Downloading 10%\rDownloading 50%\rDown')
  assert.deepEqual(lines, ['Downloading 10%', 'Downloading 50%'])
  assert.equal(rest, 'Down')
  ;[lines, rest] = appendOutput(lines, rest, 'loading 100%\n\n')
  assert.deepEqual(lines, ['Downloading 10%', 'Downloading 50%', 'Downloading 100%'])
  assert.equal(rest, '')
  ;[lines] = appendOutput([], '', Array.from({ length: 20 }, (_, i) => `line ${i}\n`).join(''))
  assert.equal(lines.length, KEPT_LINES)
  assert.equal(lines[lines.length - 1], 'line 19')
})

test('helperExitText words each helper and pkexec code', () => {
  assert.match(helperExitText(126, ''), /dismissed/)
  assert.match(helperExitText(127, ''), /not allowed/)
  assert.match(helperExitText(3, ''), /Another package manager/)
  assert.equal(
    helperExitText(2, 'lgrassroot: git is not on the list'),
    'lgrassroot: git is not on the list'
  )
  assert.match(helperExitText(2, 'unrelated'), /refused/)
  assert.match(helperExitText(1, ''), /package manager failed/)
  assert.match(helperExitText(null, ''), /stopped/)
})

test('requiredRegressed only fires for a required dependency that was ok', () => {
  const report = parseReport(withGit({ status: 'belowFloor' }))
  assert.ok(report)
  assert.equal(requiredRegressed({ git: 'ok' }, report), true)
  assert.equal(requiredRegressed({ git: 'belowFloor' }, report), false)
  assert.equal(requiredRegressed({}, report), false)
  assert.equal(requiredRegressed({ adb: 'ok' }, report), false)
})

test('nextSeen keeps an ok record until the layer completes', () => {
  const broken = parseReport(withGit({ status: 'missing' }))
  const fixed = parseReport(JSON.stringify(fixture))
  assert.ok(broken && fixed)
  assert.deepEqual(nextSeen({ git: 'ok' }, broken, false), { git: 'ok' })
  assert.deepEqual(nextSeen({ git: 'ok' }, broken, true), { git: 'missing' })
  assert.deepEqual(nextSeen({ git: 'missing' }, fixed, false), { git: 'ok' })
})

test('successText checks the result of the fresh check', () => {
  const report = parseReport(withGit({ status: 'belowFloor' }))
  assert.ok(report)
  assert.deepEqual(successText('upgrade', report.deps[0]), {
    ok: false,
    text: 'Your repositories have no newer git.'
  })
  assert.deepEqual(successText('install', report.deps[3]), { ok: true, text: 'Installed.' })
})

// Printed by lgrassd deps check --json on a Fedora 43 machine.
const realOutput =
  '{"os":{"family":"fedora","id":"fedora","version":"43"},"deps":[{"name":"git","required":true,"features":["gitPane","conflicts","rebaseForecast","biblioHistory","workspaces"],"floor":"2.45.0","path":"/usr/bin/git","version":"2.56.0","origin":{"kind":"package","package":"git-core"},"status":"ok","offer":null,"message":""},{"name":"adb","required":false,"features":["device","flutterRecording"],"floor":"","path":"/home/user/Android/Sdk/platform-tools/adb","version":"36.0.2","origin":{"kind":"manual"},"status":"ok","offer":null,"message":""},{"name":"scrcpy","required":false,"features":["device","flutterRecording"],"floor":"","path":"","version":"","origin":{"kind":"none"},"status":"missing","offer":null,"message":"scrcpy has no package in the repositories this system has enabled. Its developers publish it for Fedora through the COPR repository zeno/scrcpy."},{"name":"ffmpeg","required":false,"features":["flutterRecording"],"floor":"","path":"/usr/bin/ffmpeg","version":"7.1.5","origin":{"kind":"package","package":"ffmpeg"},"status":"ok","offer":null,"message":""},{"name":"ssh","required":false,"features":["sshChannels"],"floor":"","path":"/usr/bin/ssh","version":"10.0p2","origin":{"kind":"package","package":"openssh-clients"},"status":"ok","offer":null,"message":""}]}'

test('parseReport reads real lgrassd output', () => {
  const report = parseReport(realOutput)
  assert.ok(report)
  assert.equal(report.deps.length, 5)
  assert.deepEqual(report.deps[3].origin, { kind: 'package', package: 'ffmpeg' })
  assert.match(report.deps[2].message, /COPR/)
})
