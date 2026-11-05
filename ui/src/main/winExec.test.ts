// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// @ts-ignore
import assert from 'node:assert/strict'
import { test } from 'node:test'
import {
  envValue,
  expandEnv,
  mergeWindowsPath,
  parseRegValue,
  shimTarget,
  windowsLaunch,
  windowsShell,
  withPath
  // @ts-ignore
} from './winExec.ts'

// An in-memory Windows disk: full path to content.
function disk(files: Record<string, string>) {
  return {
    isFile: (path: string) => path in files,
    read: (path: string) => {
      if (!(path in files)) throw new Error('ENOENT')
      return files[path]
    }
  }
}

const NPM_SHIM = [
  '@ECHO off',
  'GOTO start',
  ':find_dp0',
  'SET dp0=%~dp0',
  'EXIT /b',
  ':start',
  'SETLOCAL',
  'CALL :find_dp0',
  'IF EXIST "%dp0%\\node.exe" (',
  '  SET "_prog=%dp0%\\node.exe"',
  ') ELSE (',
  '  SET "_prog=node"',
  '  SET PATHEXT=%PATHEXT:;.JS;=;%',
  ')',
  'endLocal & goto #_undefined_# 2>NUL || title %COMSPEC% & "%_prog%"  "%dp0%\\node_modules\\@acme\\agent\\cli.js" %*'
].join('\r\n')

test('env names match case-insensitively', () => {
  assert.equal(envValue({ Path: 'a' }, 'PATH'), 'a')
  assert.equal(envValue({}, 'PATH'), undefined)
})

test('an npm shim resolves to its script beside the shim', () => {
  assert.equal(
    shimTarget('C:\\npm\\agent.cmd', NPM_SHIM),
    'C:\\npm\\node_modules\\@acme\\agent\\cli.js'
  )
})

test('a shim for a native binary resolves to the exe', () => {
  const text = '@ECHO off\r\n"%~dp0\\node_modules\\@acme\\agent\\bin\\agent.exe"   %*\r\n'
  assert.equal(
    shimTarget('C:\\npm\\agent.cmd', text),
    'C:\\npm\\node_modules\\@acme\\agent\\bin\\agent.exe'
  )
})

test('a batch file with no shim line has no target', () => {
  assert.equal(shimTarget('C:\\x\\agent.cmd', '@echo off\r\necho hi\r\n'), undefined)
})

test('an exe on PATH is launched directly, found through PATHEXT', () => {
  const fs = disk({ 'C:\\bin\\claude.exe': '' })
  const launch = windowsLaunch(
    'claude',
    ['--x'],
    { PATH: 'C:\\bin', PATHEXT: '.COM;.EXE;.CMD' },
    fs
  )
  assert.deepEqual(launch, { file: 'C:\\bin\\claude.exe', args: ['--x'] })
})

test('PATH order wins over PATHEXT order', () => {
  const fs = disk({ 'C:\\a\\codex.cmd': '@echo off\r\n', 'C:\\b\\codex.exe': '' })
  const launch = windowsLaunch('codex', [], { PATH: 'C:\\a;C:\\b' }, fs)
  assert.ok('error' in launch)
})

test('an npm shim runs its script with the node beside it, never cmd.exe', () => {
  const script = 'C:\\npm\\node_modules\\@acme\\agent\\cli.js'
  const fs = disk({ 'C:\\npm\\codex.cmd': NPM_SHIM, 'C:\\npm\\node.exe': '', [script]: '' })
  const launch = windowsLaunch('codex', ['-c', '{"a":1}'], { PATH: 'C:\\npm' }, fs)
  assert.deepEqual(launch, { file: 'C:\\npm\\node.exe', args: [script, '-c', '{"a":1}'] })
})

test('an npm shim without node beside it uses node from PATH', () => {
  const script = 'C:\\npm\\node_modules\\@acme\\agent\\cli.js'
  const fs = disk({ 'C:\\npm\\codex.cmd': NPM_SHIM, [script]: '', 'C:\\node\\node.exe': '' })
  const launch = windowsLaunch('codex', [], { PATH: 'C:\\npm;C:\\node' }, fs)
  assert.deepEqual(launch, { file: 'C:\\node\\node.exe', args: [script] })
})

test('a missing agent or an unreadable shim is an error naming what to do', () => {
  const missing = windowsLaunch('claude', [], { PATH: 'C:\\bin' }, disk({}))
  assert.ok('error' in missing && missing.error.includes('Install the claude CLI'))
  const fs = disk({ 'C:\\npm\\claude.cmd': '@echo off\r\n' })
  const bad = windowsLaunch('claude', [], { PATH: 'C:\\npm' }, fs)
  assert.ok(
    'error' in bad && bad.error.includes('C:\\npm\\claude.cmd') && bad.error.includes('Reinstall')
  )
  assert.ok(!bad.error.includes('--'))
  const gone = disk({ 'C:\\npm\\claude.cmd': NPM_SHIM })
  assert.ok('error' in windowsLaunch('claude', [], { PATH: 'C:\\npm' }, gone))
})

test('the Shell entry prefers pwsh, then powershell, then ComSpec', () => {
  const both = disk({ 'C:\\ps7\\pwsh.exe': '', 'C:\\ps5\\powershell.exe': '' })
  assert.equal(windowsShell({ PATH: 'C:\\ps5;C:\\ps7' }, both), 'C:\\ps7\\pwsh.exe')
  const ps = disk({ 'C:\\ps5\\powershell.exe': '' })
  assert.equal(windowsShell({ PATH: 'C:\\ps5' }, ps), 'C:\\ps5\\powershell.exe')
  const none = disk({})
  assert.equal(
    windowsShell({ PATH: '', ComSpec: 'C:\\Windows\\cmd.exe' }, none),
    'C:\\Windows\\cmd.exe'
  )
  assert.equal(windowsShell({ PATH: '' }, none), 'cmd.exe')
})

test('reg query output and expansion', () => {
  const out =
    '\r\nHKEY_CURRENT_USER\\Environment\r\n    Path    REG_EXPAND_SZ    %USERPROFILE%\\.local\\bin;C:\\x\r\n\r\n'
  assert.equal(parseRegValue(out, 'Path'), '%USERPROFILE%\\.local\\bin;C:\\x')
  assert.equal(
    expandEnv('%USERPROFILE%\\a;%NOPE%\\b', { USERPROFILE: 'C:\\Users\\A' }),
    'C:\\Users\\A\\a;%NOPE%\\b'
  )
})

test('machine, then user, then the app PATH, without repeats', () => {
  const merged = mergeWindowsPath(
    'C:\\Windows;C:\\Git\\cmd',
    '%HOME%\\bin;c:\\windows\\',
    'C:\\Windows;C:\\dev',
    {
      HOME: 'C:\\Users\\A'
    }
  )
  assert.equal(merged, 'C:\\Windows;C:\\Git\\cmd;C:\\Users\\A\\bin;C:\\dev')
})

test('withPath replaces PATH whatever its casing', () => {
  assert.deepEqual(withPath({ Path: 'a', PATH: 'b', X: '1' }, 'c'), { X: '1', Path: 'c' })
})
