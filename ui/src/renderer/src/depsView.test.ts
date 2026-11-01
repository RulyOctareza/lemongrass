// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import type { DepEntry } from '../../preload/types'
import {
  buttonWeight,
  featuresText,
  listText,
  originText,
  osText,
  routeBlocked,
  statusText,
  tone
} from './depsView.ts'

const git: DepEntry = {
  name: 'git',
  required: true,
  features: ['gitPane', 'rebaseForecast'],
  floor: '2.45.0',
  path: '/usr/bin/git',
  version: '2.43.5',
  origin: { kind: 'package', package: 'git-core' },
  status: 'belowFloor',
  offer: { action: 'upgrade', package: 'git-core', route: 'helper' },
  message: ''
}

const adb: DepEntry = {
  ...git,
  name: 'adb',
  required: false,
  features: ['device', 'newThing'],
  floor: '',
  path: '',
  version: '',
  origin: { kind: 'none' },
  status: 'missing',
  offer: { action: 'install', package: 'android-tools', route: 'helper' }
}

test('buttonWeight follows importance', () => {
  assert.equal(buttonWeight(git), 'primary')
  assert.equal(buttonWeight(adb), 'ghost')
  assert.equal(buttonWeight({ ...adb, offer: null }), null)
})

test('the row texts read as sentences', () => {
  assert.equal(statusText(git), '2.43.5 is older than 2.45.0')
  assert.equal(statusText(adb), 'Not installed')
  assert.equal(originText(git), 'from git-core')
  assert.equal(originText({ ...git, origin: { kind: 'manual' } }), 'installed by hand')
  assert.equal(originText(adb), '')
  assert.equal(featuresText(git.features), 'Used by the Git pane and the rebase forecast.')
  assert.equal(featuresText(adb.features), 'Used by the Device pane and newThing.')
  assert.equal(listText(['a', 'b', 'c']), 'a, b and c')
  assert.equal(osText({ family: 'fedora', id: 'fedora', version: '43' }), 'Fedora 43')
})

test('tone and route', () => {
  assert.equal(tone(git), 'warn')
  assert.equal(tone(adb), 'muted')
  assert.equal(tone({ ...adb, status: 'ok' }), 'ok')
  assert.equal(routeBlocked(git.offer!), null)
  assert.ok(routeBlocked({ ...git.offer!, route: 'winget' }))
})
