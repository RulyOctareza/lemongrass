// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { fileFamilyOf, fileIconOf, folderFamilyOf, folderIconOf } from './fileIcons.ts'

test('files map to a family by extension', () => {
  assert.equal(fileFamilyOf('main.ts'), 'ts')
  assert.equal(fileFamilyOf('App.vue'), 'component')
  assert.equal(fileFamilyOf('server.go'), 'go')
  assert.equal(fileFamilyOf('style.SCSS'), 'style')
  assert.equal(fileFamilyOf('data.json'), 'data')
  assert.equal(fileFamilyOf('logo.png'), 'image')
  assert.equal(fileFamilyOf('dist.tar'), 'archive')
  assert.equal(fileFamilyOf('notes.md'), 'doc')
  assert.equal(fileFamilyOf('run.sh'), 'shell')
  assert.equal(fileFamilyOf('unknown.xyz'), 'file')
  assert.equal(fileFamilyOf('noext'), 'file')
})

test('special names win over their extension', () => {
  assert.equal(fileFamilyOf('package-lock.json'), 'lock')
  assert.equal(fileFamilyOf('yarn.lock'), 'lock')
  assert.equal(fileFamilyOf('go.sum'), 'lock')
  assert.equal(fileFamilyOf('Dockerfile'), 'config')
  assert.equal(fileFamilyOf('Makefile'), 'config')
  assert.equal(fileFamilyOf('.gitignore'), 'config')
  assert.equal(fileFamilyOf('.env.local'), 'config')
  assert.equal(fileFamilyOf('tsconfig.node.json'), 'config')
  assert.equal(fileFamilyOf('vite.config.ts'), 'config')
  assert.equal(fileFamilyOf('README'), 'doc')
  assert.equal(fileFamilyOf('LICENSE.txt'), 'doc')
  assert.equal(fileFamilyOf('index.d.ts'), 'ts')
})

test('special folders map to a family, others are plain', () => {
  assert.equal(folderFamilyOf('src'), 'src')
  assert.equal(folderFamilyOf('__tests__'), 'test')
  assert.equal(folderFamilyOf('node_modules'), 'deps')
  assert.equal(folderFamilyOf('.github'), 'tooling')
  assert.equal(folderFamilyOf('dist'), 'build')
  assert.equal(folderFamilyOf('components'), 'folder')
})

test('every icon colors through a token and has shapes', () => {
  for (const name of ['a.ts', 'a.png', 'a.xyz', 'yarn.lock']) {
    const icon = fileIconOf(name)
    assert.match(icon.color, /^var\(--color-icon-[a-z-]+\)$/)
    assert.ok(icon.shapes.length > 0)
  }
  assert.equal(folderIconOf('lib', false).color, 'var(--color-icon-folder-src)')
  assert.equal(folderIconOf('misc', true).color, 'var(--color-icon-folder)')
  assert.notDeepEqual(folderIconOf('misc', true).shapes, folderIconOf('misc', false).shapes)
})
