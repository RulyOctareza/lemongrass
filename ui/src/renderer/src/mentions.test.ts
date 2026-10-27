// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { bodyParts } from './mentions.ts'

const members = [
  { tabId: '35546ed4-1111-4222-8333-444455556666', label: 'lead' },
  { tabId: 'e7f244ea-d54c-4226-9156-b5429737b257', label: 'checker' }
]

test('a short mention shows the member label', () => {
  const parts = bodyParts('!>>35546ed4<<! checker here', members)
  assert.deepEqual(parts[0], { text: '@lead', mention: true })
  assert.equal(parts[1].text, ' checker here')
})

test('a full tab id mention shows the member label', () => {
  const parts = bodyParts('hi !>>E7F244EA-D54C-4226-9156-B5429737B257<<!', members)
  assert.deepEqual(parts[1], { text: '@checker', mention: true })
})

test('an unknown mention stays as written', () => {
  const parts = bodyParts('!>>deadbeef<<!', members)
  assert.deepEqual(parts[0], { text: '!>>deadbeef<<!', mention: false })
})

test('a mention with a loose or missing closing shows the member label', () => {
  for (const body of ['!>>35546ed4<< checker here', '!>>35546ed4<! checker here', '!>>35546ed4< checker here', '!>>35546ed4 checker here']) {
    const parts = bodyParts(body, members)
    assert.deepEqual(parts[0], { text: '@lead', mention: true }, body)
    assert.equal(parts[1].text, ' checker here', body)
  }
})

test('a mention followed by punctuation keeps the punctuation', () => {
  const parts = bodyParts('thanks !>>35546ed4, done', members)
  assert.deepEqual(parts[1], { text: '@lead', mention: true })
  assert.equal(parts[2].text, ', done')
})
