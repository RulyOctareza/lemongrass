// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import assert from 'node:assert/strict'
import { test } from 'node:test'
import { mkdtempSync, writeFileSync } from 'fs'
import * as net from 'net'
import { tmpdir } from 'os'
import { join } from 'path'
import { homeTag, verifyGreeting } from './ipcAddress.ts'

// verifyGreeting runs only on Windows in the app; a unix socket stands in for the pipe here.
function greetingServer(
  greeting: string,
  reply: string
): Promise<{ path: string; server: net.Server }> {
  const dir = mkdtempSync(join(tmpdir(), 'lgipc-'))
  const path = join(dir, 's.sock')
  const server = net.createServer((conn) => {
    conn.write(greeting)
    conn.once('data', () => conn.end(reply))
  })
  return new Promise((resolve) => server.listen(path, () => resolve({ path, server })))
}

function keyDir(token: string): string {
  const dir = mkdtempSync(join(tmpdir(), 'lgkey-'))
  writeFileSync(join(dir, 'vault.ipckey'), token + '\n')
  return dir
}

function connect(path: string): Promise<net.Socket> {
  return new Promise((resolve, reject) => {
    const s = net.createConnection(path)
    s.once('connect', () => resolve(s))
    s.once('error', reject)
  })
}

test('a server greeting with the token on disk is accepted and its reply reaches the caller', async () => {
  const keys = keyDir('abc')
  const { path, server } = await greetingServer('\0lgipc abc\n', '{"ok":true}\n')
  const socket = await verifyGreeting(await connect(path), 'vault', keys)
  const reply = await new Promise<string>((resolve) => {
    const chunks: Buffer[] = []
    socket.on('data', (c: Buffer) => chunks.push(c))
    socket.on('close', () => resolve(Buffer.concat(chunks).toString()))
    socket.write('{"op":"x"}\n')
  })
  assert.equal(reply, '{"ok":true}\n')
  server.close()
})

test('a server with the wrong token is dropped', async () => {
  const keys = keyDir('abc')
  const { path, server } = await greetingServer('\0lgipc zzz\n', '{"ok":true}\n')
  await assert.rejects(verifyGreeting(await connect(path), 'vault', keys), { code: 'EIPCPEER' })
  server.close()
})

test('a server that does not greet is dropped', async () => {
  const keys = keyDir('abc')
  const { path, server } = await greetingServer('{"ok":true}\n', '')
  await assert.rejects(verifyGreeting(await connect(path), 'vault', keys), { code: 'EIPCPEER' })
  server.close()
})

test('the profile home has no pipe tag, another home a tag of its lowercased dir', () => {
  assert.equal(homeTag('C:\\Users\\a\\lg', 'C:\\Users\\a'), '')
  assert.equal(homeTag('C:\\Users\\a\\lg', 'c:\\users\\A'), '')
  assert.equal(homeTag('C:\\Tmp\\test-home\\lg', 'C:\\Users\\a'), '-h964c5174')
})
