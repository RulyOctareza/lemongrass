// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import {
  compareVersions,
  dmgAssetName,
  isNewer,
  newestRelease,
  rpmAssetName,
  sha256For,
  versionFromTag
} from './updateVersion.ts'

test('compares the numeric parts', () => {
  assert.equal(compareVersions('0.2.0', '0.1.9'), 1)
  assert.equal(compareVersions('0.10.0', '0.9.0'), 1)
  assert.equal(compareVersions('1.0.0', '1.0.0'), 0)
  assert.equal(compareVersions('v0.2.0', '0.2.1'), -1)
})

test('a final release sorts after its prereleases', () => {
  assert.equal(compareVersions('0.2.0', '0.2.0-rc.1'), 1)
  assert.equal(compareVersions('0.2.0-rc.2', '0.2.0-rc.1'), 1)
  assert.equal(compareVersions('0.2.0-rc.10', '0.2.0-rc.9'), 1)
  assert.equal(compareVersions('0.2.0-rc.1', '0.2.0-rc'), 1)
  assert.equal(compareVersions('0.2.0-1', '0.2.0-rc'), -1)
})

test('a version that is not semver is never offered an update', () => {
  assert.equal(compareVersions('0.2.0', 'local'), null)
  assert.equal(isNewer('0.2.0', 'local'), false)
  assert.equal(isNewer('0.2.0', '0.1.0'), true)
  assert.equal(isNewer('0.2.0', '0.2.0'), false)
})

test('names the release assets', () => {
  assert.equal(versionFromTag('v0.2.0'), '0.2.0')
  assert.equal(rpmAssetName('0.2.0-rc.1'), 'lemongrass-0.2.0-rc.1.x86_64.rpm')
  assert.equal(dmgAssetName('0.4.0-rc.2', 'arm64'), 'lemongrass-0.4.0-rc.2-arm64.dmg')
  assert.equal(dmgAssetName('0.4.0', 'x64'), 'lemongrass-0.4.0-x64.dmg')
})

test('the prerelease channel picks the highest version, not the newest entry', () => {
  const releases = [
    { tag_name: 'v0.3.1', draft: false },
    { tag_name: 'v0.4.0-rc.3', draft: false },
    { tag_name: 'v0.4.0-rc.2', draft: false },
    { tag_name: 'v0.5.0', draft: true },
    { tag_name: 'nightly', draft: false }
  ]
  assert.equal(newestRelease(releases)?.tag_name, 'v0.4.0-rc.3')
  assert.equal(newestRelease([...releases, { tag_name: 'v0.4.0' }])?.tag_name, 'v0.4.0')
  assert.equal(newestRelease([]), null)
})

test('reads one file from SHA256SUMS', () => {
  const digest = 'A'.repeat(64)
  const sums = `${'b'.repeat(64)}  other.rpm\n${digest} *lemongrass-0.2.0.x86_64.rpm\n`
  assert.equal(sha256For(sums, 'lemongrass-0.2.0.x86_64.rpm'), 'a'.repeat(64))
  assert.equal(sha256For(sums, 'missing.rpm'), null)
})
