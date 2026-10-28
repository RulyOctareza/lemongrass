// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { test } from 'node:test'
import assert from 'node:assert/strict'
import { composeConnectionString, parseConnectionString } from './connectionString.ts'

const base = {
  engine: 'mysql' as const,
  host: 'db-internal',
  port: '3306',
  user: 'app',
  password: 'p@ss word',
  database: 'shop',
  sshHost: '',
  query: ''
}

test('a connection without an ssh host has no lgrass_ssh param', () => {
  assert.equal(composeConnectionString(base), 'mysql://app:p%40ss%20word@db-internal:3306/shop')
})

test('the ssh host is appended after the preserved query', () => {
  assert.equal(
    composeConnectionString({ ...base, sshHost: 'bastion-prod' }),
    'mysql://app:p%40ss%20word@db-internal:3306/shop?lgrass_ssh=bastion-prod'
  )
  assert.equal(
    composeConnectionString({ ...base, sshHost: ' bastion-prod ', query: '?parseTime=true' }),
    'mysql://app:p%40ss%20word@db-internal:3306/shop?parseTime=true&lgrass_ssh=bastion-prod'
  )
})

test('parsing takes the ssh host out of the preserved query', () => {
  const parsed = parseConnectionString(
    'postgres://app:pw@db-internal:5432/shop?sslmode=disable&lgrass_ssh=bastion-prod'
  )
  assert.equal(parsed.sshHost, 'bastion-prod')
  assert.equal(parsed.query, '?sslmode=disable')
  assert.equal(parsed.host, 'db-internal')
})

test('a query without the ssh host is kept byte for byte', () => {
  const parsed = parseConnectionString('mysql://app:pw@db:3306/shop?tls=skip-verify&x=a%20b')
  assert.equal(parsed.sshHost, '')
  assert.equal(parsed.query, '?tls=skip-verify&x=a%20b')
})

test('compose after parse gives back the stored string', () => {
  const stored =
    'mysql://app:p%40ss%20word@db-internal:3306/shop?parseTime=true&lgrass_ssh=bastion-prod'
  assert.equal(composeConnectionString(parseConnectionString(stored)), stored)
})
