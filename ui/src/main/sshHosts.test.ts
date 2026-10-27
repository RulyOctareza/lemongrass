// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { parseResolvedHost, parseSSHConfigHosts, type ConfigFiles } from './sshHosts.ts'

const home = '/home/u'

function fakeFiles(tree: Record<string, string>): ConfigFiles {
  return {
    read: (path) => tree[path] ?? null,
    list: (dir) =>
      Object.keys(tree)
        .filter((p) => p.startsWith(dir + '/') && !p.slice(dir.length + 1).includes('/'))
        .map((p) => p.slice(dir.length + 1))
  }
}

test('concrete hosts are listed in file order and patterns are skipped', () => {
  const files = fakeFiles({
    '/home/u/.ssh/config': [
      '# comment',
      'Host *',
      '  ServerAliveInterval 30',
      'Host bastion-prod bastion-staging',
      '  HostName 10.0.0.5',
      'Host *.internal !secret db-?',
      'host=jump-1',
      'Match host foo',
      'Host bastion-prod'
    ].join('\n')
  })
  assert.deepEqual(parseSSHConfigHosts('/home/u/.ssh/config', home, files), [
    'bastion-prod',
    'bastion-staging',
    'jump-1'
  ])
})

test('Include is followed relative to the ssh directory, with a glob and a tilde', () => {
  const files = fakeFiles({
    '/home/u/.ssh/config': 'Include config.d/*.conf ~/extra/hosts "/etc/ssh/team"\nHost local-a',
    '/home/u/.ssh/config.d/b.conf': 'Host from-b',
    '/home/u/.ssh/config.d/a.conf': 'Host from-a',
    '/home/u/.ssh/config.d/notes.txt': 'Host ignored',
    '/home/u/extra/hosts': 'Host from-extra',
    '/etc/ssh/team': 'Host from-team'
  })
  assert.deepEqual(parseSSHConfigHosts('/home/u/.ssh/config', home, files), [
    'from-a',
    'from-b',
    'from-extra',
    'from-team',
    'local-a'
  ])
})

test('a missing config or an include loop gives what can be read', () => {
  assert.deepEqual(parseSSHConfigHosts('/home/u/.ssh/config', home, fakeFiles({})), [])
  const loop = fakeFiles({
    '/home/u/.ssh/config': 'Include config\nHost only-once'
  })
  assert.deepEqual(parseSSHConfigHosts('/home/u/.ssh/config', home, loop), ['only-once'])
})

test('ssh -G output gives the resolved target', () => {
  const output = 'host bastion-prod\nuser dev\nhostname 10.0.0.5\nport 2222\nproxyjump jump-1\n'
  assert.deepEqual(parseResolvedHost(output), {
    hostname: '10.0.0.5',
    user: 'dev',
    port: '2222',
    proxyJump: 'jump-1'
  })
  assert.equal(parseResolvedHost('hostname h\nproxyjump none\n').proxyJump, '')
})
