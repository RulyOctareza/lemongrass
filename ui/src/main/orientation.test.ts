import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync, readFileSync, writeFileSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
import { orientationLaunch } from './orientation.ts'

function homeWith(...roles: string[]): string {
  const home = mkdtempSync(join(tmpdir(), 'orientation-'))
  const dir = join(home, '.lemongrass', 'orientation')
  mkdirSync(dir, { recursive: true })
  for (const role of roles) writeFileSync(join(dir, `${role}.md`), `${role} text\n`)
  return home
}

function projectWithLaws(summary: string): string {
  const cwd = mkdtempSync(join(tmpdir(), 'project-'))
  mkdirSync(join(cwd, 'biblio', 'laws'), { recursive: true })
  writeFileSync(join(cwd, 'biblio', 'laws', 'summary.md'), summary)
  return cwd
}

test('a plain Claude tab gets the tab orientation', () => {
  const home = homeWith('tab', 'thinker')
  assert.deepEqual(orientationLaunch('claude', 'tab', home), {
    args: ['--append-system-prompt-file', join(home, '.lemongrass', 'orientation', 'tab.md')],
    env: {}
  })
})

test('a Claude thinker gets the thinker orientation', () => {
  const home = homeWith('tab', 'thinker')
  assert.deepEqual(orientationLaunch('claude', 'thinker', home), {
    args: ['--append-system-prompt-file', join(home, '.lemongrass', 'orientation', 'thinker.md')],
    env: {}
  })
})

test('a project with laws gets the orientation followed by its laws summary', () => {
  const home = homeWith('tab')
  const cwd = projectWithLaws('# Laws summary\n\n- a rule\n')
  const launch = orientationLaunch('claude', 'tab', home, cwd)
  assert.equal(launch.args[0], '--append-system-prompt-file')
  assert.equal(readFileSync(launch.args[1], 'utf8'), 'tab text\n\n# Laws summary\n\n- a rule\n')
  assert.deepEqual(launch.env, { LGRASS_LAWS_IN_PROMPT: '1' })
})

test('two projects get separate prompt files and one project keeps its path', () => {
  const home = homeWith('tab')
  const first = projectWithLaws('# Laws summary\n\n- one\n')
  const second = projectWithLaws('# Laws summary\n\n- two\n')
  const a = orientationLaunch('claude', 'tab', home, first).args[1]
  const b = orientationLaunch('claude', 'tab', home, second).args[1]
  assert.notEqual(a, b)
  assert.equal(orientationLaunch('claude', 'tab', home, first).args[1], a)
})

test('a project without laws gets the plain orientation and no flag env', () => {
  const home = homeWith('tab')
  const cwd = mkdtempSync(join(tmpdir(), 'project-'))
  assert.deepEqual(orientationLaunch('claude', 'tab', home, cwd), {
    args: ['--append-system-prompt-file', join(home, '.lemongrass', 'orientation', 'tab.md')],
    env: {}
  })
})

test('other agents get no orientation', () => {
  const home = homeWith('tab', 'thinker')
  const cwd = projectWithLaws('# Laws summary\n')
  for (const command of ['codex', 'agent', 'zsh']) {
    assert.deepEqual(orientationLaunch(command, 'tab', home, cwd), { args: [], env: {} })
  }
})

test('a missing orientation file adds nothing', () => {
  const cwd = projectWithLaws('# Laws summary\n')
  assert.deepEqual(orientationLaunch('claude', 'thinker', homeWith('tab'), cwd), {
    args: [],
    env: {}
  })
})
