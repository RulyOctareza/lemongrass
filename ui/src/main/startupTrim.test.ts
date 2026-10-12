// @ts-ignore
import assert from 'node:assert/strict'
import { mkdirSync, mkdtempSync } from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { test } from 'node:test'
// @ts-ignore
import { startupTrimArgs, startupTrimEnv } from './startupTrim.ts'

function home(withPlugin: boolean): string {
  const dir = mkdtempSync(join(tmpdir(), 'trim-'))
  if (withPlugin)
    mkdirSync(join(dir, '.lemongrass', 'plugin', 'lemongrass-tools'), { recursive: true })
  return dir
}

test('a Claude tab gets the trim env', () => {
  assert.deepEqual(startupTrimEnv('claude'), {
    CLAUDE_CODE_DISABLE_BUNDLED_SKILLS: '1',
    ENABLE_CLAUDEAI_MCP_SERVERS: 'false'
  })
})

test('a Claude tab gets the denied tools and the plugin when it is installed', () => {
  const dir = home(true)
  assert.deepEqual(startupTrimArgs('claude', dir), [
    '--disallowedTools',
    'SendFeedback ShareOnboardingGuide ReportFindings EnterPlanMode ExitPlanMode',
    '--plugin-dir',
    join(dir, '.lemongrass', 'plugin', 'lemongrass-tools')
  ])
})

test('a missing plugin adds no plugin flag', () => {
  assert.deepEqual(startupTrimArgs('claude', home(false)), [
    '--disallowedTools',
    'SendFeedback ShareOnboardingGuide ReportFindings EnterPlanMode ExitPlanMode'
  ])
})

test('other agents get no trim', () => {
  for (const command of ['codex', 'agent', 'zsh']) {
    assert.deepEqual(startupTrimEnv(command), {})
    assert.deepEqual(startupTrimArgs(command, home(true)), [])
  }
})
