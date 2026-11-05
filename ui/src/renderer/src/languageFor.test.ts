// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { languageIdFor } from './languageFor.ts'

test('an extension picks its language, case-insensitive', () => {
  assert.equal(languageIdFor('cmd/lgrass/main.go'), 'go')
  assert.equal(languageIdFor('ui/src/App.vue'), 'vue')
  assert.equal(languageIdFor('ui/scripts/afterPack.mjs'), 'javascript')
  assert.equal(languageIdFor('src/View.TSX'), 'tsx')
  assert.equal(languageIdFor('.github/workflows/release.yml'), 'yaml')
  assert.equal(languageIdFor('scripts/dev.sh'), 'shell')
})

test('a whole name wins over an extension and covers Dockerfile variants', () => {
  assert.equal(languageIdFor('build/Containerfile'), 'dockerfile')
  assert.equal(languageIdFor('Dockerfile'), 'dockerfile')
  assert.equal(languageIdFor('docker/Dockerfile.dev'), 'dockerfile')
})

test('an unknown extension, a dotfile or no extension is plain text', () => {
  assert.equal(languageIdFor('go.sum'), null)
  assert.equal(languageIdFor('.gitignore'), null)
  assert.equal(languageIdFor('Makefile'), null)
  assert.equal(languageIdFor('LICENSE'), null)
  assert.equal(languageIdFor('notes.txt'), null)
})

test('a name that matches an object prototype key is plain text', () => {
  assert.equal(languageIdFor('a.constructor'), null)
  assert.equal(languageIdFor('toString'), null)
})
