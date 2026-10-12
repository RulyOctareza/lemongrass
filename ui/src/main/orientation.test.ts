import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { orientationArgs } from './orientation.ts'

function homeWith(...roles: string[]): string {
  const home = mkdtempSync(join(tmpdir(), 'orientation-'))
  const dir = join(home, '.lemongrass', 'orientation')
  mkdirSync(dir, { recursive: true })
  for (const role of roles) writeFileSync(join(dir, `${role}.md`), 'text')
  return home
}

test('a plain Claude tab gets the tab orientation', () => {
  const home = homeWith('tab', 'thinker')
  assert.deepEqual(orientationArgs('claude', 'tab', home), [
    '--append-system-prompt-file',
    join(home, '.lemongrass', 'orientation', 'tab.md')
  ])
})

test('a Claude thinker gets the thinker orientation', () => {
  const home = homeWith('tab', 'thinker')
  assert.deepEqual(orientationArgs('claude', 'thinker', home), [
    '--append-system-prompt-file',
    join(home, '.lemongrass', 'orientation', 'thinker.md')
  ])
})

test('other agents get no orientation', () => {
  const home = homeWith('tab', 'thinker')
  for (const command of ['codex', 'agent', 'zsh']) {
    assert.deepEqual(orientationArgs(command, 'tab', home), [])
    assert.deepEqual(orientationArgs(command, 'thinker', home), [])
  }
})

test('a missing orientation file adds no flag', () => {
  const home = homeWith('tab')
  assert.deepEqual(orientationArgs('claude', 'thinker', home), [])
  assert.deepEqual(orientationArgs('claude', 'tab', homeWith()), [])
})
