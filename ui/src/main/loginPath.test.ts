// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
// @ts-ignore
import { extractPath, mergePath } from './loginPath.ts'

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
