// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  redactBody,
  redactHeaders,
  redactText,
  redactUrl,
  scrubText,
  secretFieldValues
} from './redact.ts'
import type { RedactHints } from './types.ts'

const header: RedactHints = {
  tokenPlacement: { Kind: 'header', Name: 'X-Api-Token', Prefix: '' },
  tokenPath: 'data.jwt'
}
const query: RedactHints = {
  tokenPlacement: { Kind: 'query', Name: 'acc', Prefix: '' },
  tokenPath: ''
}
const none: RedactHints = { tokenPlacement: null, tokenPath: '' }

test('headers: credential names are redacted by pattern, others kept', () => {
  const out = redactHeaders(
    {
      authorization: 'Bearer abc',
      Cookie: 'a=b',
      'Set-Cookie': 'c=d',
      'x-api-token': 't',
      'X-CSRF-Token': 'c',
      'X-Auth-Token': 'a',
      'X-Api-Key': 'k',
      Accept: 'json',
      'Content-Type': 'application/json'
    },
    header
  )
  assert.deepEqual(out, {
    authorization: '[redacted]',
    Cookie: '[redacted]',
    'Set-Cookie': '[redacted]',
    'x-api-token': '[redacted]',
    'X-CSRF-Token': '[redacted]',
    'X-Auth-Token': '[redacted]',
    'X-Api-Key': '[redacted]',
    Accept: 'json',
    'Content-Type': 'application/json'
  })
})

test('headers: the placed header is redacted even when its name looks harmless', () => {
  const hints: RedactHints = {
    tokenPlacement: { Kind: 'header', Name: 'X-Who', Prefix: '' },
    tokenPath: ''
  }
  assert.equal(redactHeaders({ 'X-Who': 't' }, hints)['X-Who'], '[redacted]')
  assert.equal(redactHeaders({ 'X-Who': 't' }, none)['X-Who'], 't')
})

test('url: sensitive parameters are replaced in the query', () => {
  assert.equal(
    redactUrl('http://h/x?a=1&access_token=S&code=C&state=T&acc=P&b=2', query),
    'http://h/x?a=1&access_token=[redacted]&code=[redacted]&state=[redacted]&acc=[redacted]&b=2'
  )
})

test('url: an SSO landing hides the fragment token too, and a page query after a hash route', () => {
  assert.equal(
    redactUrl('http://h/cb?code=C#access_token=A&id_token=I&token_type=bearer', none),
    'http://h/cb?code=[redacted]#access_token=[redacted]&id_token=[redacted]&token_type=[redacted]'
  )
  assert.equal(
    redactUrl('http://h/#/cb?refresh_token=R&tab=2', none),
    'http://h/#/cb?refresh_token=[redacted]&tab=2'
  )
  assert.equal(redactUrl('http://h/orders?page=2#/row/5', none), 'http://h/orders?page=2#/row/5')
})

test('body: JSON keys that name a credential are hidden at any depth, the token path too', () => {
  const out = JSON.parse(
    redactBody(
      '{"data":{"jwt":"J","id":1,"nested":[{"refreshToken":"R","ok":true}]},"accessToken":"A","client_secret":"S","name":"n","code":"E_NOT_FOUND"}',
      header
    )
  )
  assert.deepEqual(out, {
    data: { jwt: '[redacted]', id: 1, nested: [{ refreshToken: '[redacted]', ok: true }] },
    accessToken: '[redacted]',
    client_secret: '[redacted]',
    name: 'n',
    code: 'E_NOT_FOUND'
  })
})

test('body: a token path with an innocent key name is still hidden', () => {
  const hints: RedactHints = { tokenPlacement: null, tokenPath: 'data.value' }
  assert.equal(
    redactBody('{"data":{"value":"V","id":2}}', hints),
    '{"data":{"value":"[redacted]","id":2}}'
  )
})

test('body: a form body hides credential fields only', () => {
  assert.equal(
    redactBody('grant_type=refresh_token&refresh_token=R&client_secret=S&username=u', none),
    'grant_type=refresh_token&refresh_token=[redacted]&client_secret=[redacted]&username=u'
  )
})

test('body: free text keeps its words and loses credential pairs', () => {
  assert.equal(redactBody('not json', none), 'not json')
  assert.equal(
    redactBody('oops "api_key": "K123", retry', none),
    'oops "api_key": "[redacted]", retry'
  )
})

test('text: bearer values, JWTs, addresses and loose pairs are scrubbed', () => {
  assert.equal(redactText('sent Bearer abcdefgh12345 ok', none), 'sent Bearer [redacted] ok')
  assert.equal(
    redactText('jwt eyJhbGciOiJI.eyJzdWIiOiJ4.SIG_nature end', none),
    'jwt [redacted] end'
  )
  assert.equal(
    redactText('failed (http://h/cb?code=C&x=1#access_token=A)', none),
    'failed (http://h/cb?code=[redacted]&x=1#access_token=[redacted])'
  )
  assert.equal(
    redactText('retry with refresh_token=abc123 now', none),
    'retry with refresh_token=[redacted] now'
  )
})

test('scrub: the user login values go in plain, URL encoded and JSON escaped forms', () => {
  const secrets = ['p@ss word"1', 'ab']
  const text = 'a p@ss word"1 b p%40ss%20word%221 c p@ss word\\"1 d ab'
  assert.equal(scrubText(text, secrets, none), 'a [redacted] b [redacted] c [redacted] d ab')
})

test('scrub: the longer value wins when one contains another', () => {
  assert.equal(scrubText('hunter22 hunter', ['hunter', 'hunter22'], none), '[redacted] [redacted]')
})

test('secret field values: only secret looking keys, never the user name or an email identity', () => {
  const fields = {
    email: 'dave@test.local',
    password: 'pw1',
    otp_code: '123456',
    tenant: 'acme',
    pin: 'dave@test.local'
  }
  assert.deepEqual(secretFieldValues(fields, ['dave@test.local']), ['pw1', '123456'])
  assert.deepEqual(secretFieldValues({ username: 'alice', token_value: 'T' }, ['alice']), ['T'])
})
