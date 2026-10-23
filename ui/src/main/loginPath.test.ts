// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
// @ts-ignore
import { extractMarked, extractPath, mergePath, pickSSHAuthSock } from './loginPath.ts'

test('the PATH between the markers is read past shell banners', () => {
  const output = 'welcome\n__LGRASS_PATH__/a/bin:/usr/bin__LGRASS_PATH__\nbye'
  assert.equal(extractPath(output), '/a/bin:/usr/bin')
})

test('output without both markers gives no PATH', () => {
  assert.equal(extractPath('no markers here'), null)
  assert.equal(extractPath('__LGRASS_PATH__/a/bin'), null)
  assert.equal(extractPath('__LGRASS_PATH____LGRASS_PATH__'), null)
})

test('login entries come first and duplicates are dropped', () => {
  assert.equal(
    mergePath('/home/u/.local/bin:/usr/bin', '/usr/bin:/bin', ['/opt/homebrew/bin', '/bin']),
    '/home/u/.local/bin:/usr/bin:/bin:/opt/homebrew/bin'
  )
})

test('a failed login read keeps the current PATH plus the fallbacks', () => {
  assert.equal(
    mergePath(null, '/usr/bin:/bin', ['/home/u/.local/bin']),
    '/usr/bin:/bin:/home/u/.local/bin'
  )
})

test('SSH_AUTH_SOCK is read from its own markers beside the PATH', () => {
  const output =
    'motd\n__LGRASS_PATH__/usr/bin__LGRASS_PATH____LGRASS_SSH_AUTH_SOCK__/run/user/1000/agent.sock__LGRASS_SSH_AUTH_SOCK__'
  assert.equal(extractPath(output), '/usr/bin')
  assert.equal(extractMarked(output, '__LGRASS_SSH_AUTH_SOCK__'), '/run/user/1000/agent.sock')
  assert.equal(
    extractMarked('__LGRASS_SSH_AUTH_SOCK____LGRASS_SSH_AUTH_SOCK__', '__LGRASS_SSH_AUTH_SOCK__'),
    null
  )
})

test('an SSH_AUTH_SOCK the app already has is never replaced', () => {
  assert.equal(pickSSHAuthSock('/run/desktop.sock', '/tmp/shell.sock'), '/run/desktop.sock')
  assert.equal(pickSSHAuthSock(undefined, '/tmp/shell.sock'), '/tmp/shell.sock')
  assert.equal(pickSSHAuthSock('', '/tmp/shell.sock'), '/tmp/shell.sock')
  assert.equal(pickSSHAuthSock(undefined, null), null)
})
