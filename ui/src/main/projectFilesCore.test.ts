// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { after, test } from 'node:test'
import assert from 'node:assert/strict'
import { execFileSync } from 'node:child_process'
import {
  existsSync,
  mkdirSync,
  mkdtempSync,
  readFileSync,
  realpathSync,
  rmSync,
  symlinkSync,
  writeFileSync
} from 'node:fs'
import { tmpdir } from 'node:os'
import { join } from 'node:path'
import { readEnv, run } from './gitRun.ts'
import {
  copyEntries,
  copyName,
  createFile,
  createFolder,
  duplicateEntry,
  imagePath,
  isSubsequence,
  isValidName,
  listDir,
  moveEntries,
  readProjectFile,
  renameEntry,
  resolveEntry,
  resolveReal,
  searchProject,
  type ProjectGit
} from './projectFilesCore.ts'

process.env.GIT_CONFIG_GLOBAL = '/dev/null'
process.env.GIT_CONFIG_NOSYSTEM = '1'

const scratch = realpathSync(mkdtempSync(join(tmpdir(), 'lg-projectfiles-')))
after(() => rmSync(scratch, { recursive: true, force: true }))

let projects = 0
function project(files: Record<string, string>, gitignore?: string): string {
  const root = join(scratch, `p${(projects += 1)}`)
  mkdirSync(root)
  for (const [path, content] of Object.entries(files)) {
    mkdirSync(join(root, path, '..'), { recursive: true })
    writeFileSync(join(root, path), content)
  }
  if (gitignore !== undefined) {
    writeFileSync(join(root, '.gitignore'), gitignore)
    execFileSync('git', ['init', '-q'], { cwd: root })
  }
  return root
}

const gitIn =
  (root: string): ProjectGit =>
  (args, input) =>
    run(root, args, { env: { ...readEnv, GIT_LITERAL_PATHSPECS: '0' }, input })

test('confinement refuses escapes, hidden folders and outside symlinks', () => {
  const outside = project({ 'secret.txt': 's' })
  const root = project({ 'src/a.ts': 'a', 'biblio/x.md': 'x', '.git/config': 'c' })
  symlinkSync(join(outside, 'secret.txt'), join(root, 'link.txt'))
  symlinkSync(join(root, 'src'), join(root, 'srclink'))
  assert.equal(resolveReal(root, '../p1/secret.txt'), null)
  assert.equal(resolveReal(root, '/etc/passwd'), null)
  assert.equal(resolveReal(root, 'biblio/x.md'), null)
  assert.equal(resolveReal(root, '.git/config'), null)
  assert.equal(resolveReal(root, 'src/../biblio/x.md'), null)
  assert.equal(resolveReal(root, 'link.txt'), null)
  assert.equal(resolveReal(root, 'srclink/a.ts'), join(root, 'src', 'a.ts'))
  assert.equal(resolveReal(root, ''), root)
  // The link itself may be renamed or trashed, its target is never reached.
  assert.equal(resolveEntry(root, 'link.txt'), join(root, 'link.txt'))
  assert.equal(resolveEntry(root, ''), null)
  assert.equal(readProjectFile(root, 'link.txt'), null)
})

test('names', () => {
  assert.equal(isValidName('a.ts', true), true)
  for (const bad of ['', ' ', '.', '..', 'a/b', 'a\0b'])
    assert.equal(isValidName(bad, false), false)
  assert.equal(isValidName('biblio', true), false)
  assert.equal(isValidName('.GIT', true), false)
  assert.equal(isValidName('biblio', false), true)
  const taken = new Set(['a copy.ts'])
  assert.equal(
    copyName('a.ts', (n) => taken.has(n)),
    'a copy 2.ts'
  )
  assert.equal(
    copyName('.env', () => false),
    '.env copy'
  )
  assert.equal(
    copyName('src', () => false),
    'src copy'
  )
})

test('subsequence filter', () => {
  assert.equal(isSubsequence('pfc', 'ui/src/main/projectFilesCore.ts'), true)
  assert.equal(isSubsequence('PF core', 'ui/src/main/projectFilesCore.ts'), true)
  assert.equal(isSubsequence('zz', 'ui/src/main/projectFilesCore.ts'), false)
})

test('list hides .git and biblio, sorts folders first and marks ignored', async () => {
  const root = project(
    {
      'b.ts': 'b',
      'A.ts': 'a',
      'z/x.ts': 'x',
      'out.log': 'l',
      'node_modules/r/i.js': 'i',
      'biblio/t.md': 't'
    },
    'node_modules/\n*.log\n'
  )
  const result = await listDir(root, '', gitIn(root))
  assert.ok('entries' in result)
  assert.deepEqual(
    result.entries.map((e) => [e.name, e.type, e.ignored]),
    [
      ['node_modules', 'dir', true],
      ['z', 'dir', false],
      ['.gitignore', 'file', false],
      ['A.ts', 'file', false],
      ['b.ts', 'file', false],
      ['out.log', 'file', true]
    ]
  )
  assert.equal(result.entries.find((e) => e.name === 'b.ts')?.sizeBytes, 1)
  const inside = await listDir(root, 'node_modules/r', gitIn(root))
  assert.ok('entries' in inside && inside.entries[0].ignored)
  assert.deepEqual(await listDir(root, 'biblio', gitIn(root)), { error: 'outside' })
  assert.deepEqual(await listDir(root, 'nope', gitIn(root)), { error: 'notFound' })
})

test('list outside a git repository marks nothing ignored', async () => {
  const root = project({ 'a.ts': 'a' })
  const result = await listDir(root, '', gitIn(root))
  assert.ok('entries' in result)
  assert.equal(result.entries[0].ignored, false)
})

test('search skips ignored folders unless the query names one', async () => {
  const root = project(
    { 'src/app.ts': 'a', 'node_modules/react/index.js': 'r', 'biblio/app.md': 'b' },
    'node_modules/\nbiblio/\n'
  )
  const plain = await searchProject(root, 'index', gitIn(root))
  assert.deepEqual(plain.paths, [])
  const app = await searchProject(root, 'app', gitIn(root))
  assert.deepEqual(app.paths, [{ path: 'src/app.ts', type: 'file' }])
  const named = await searchProject(root, 'node_modules/index', gitIn(root))
  assert.deepEqual(named.paths, [
    { path: 'node_modules/react/index.js', type: 'file', ignored: true }
  ])
  assert.equal(named.truncated, false)
  assert.deepEqual((await searchProject(root, '  ', gitIn(root))).paths, [])
})

test('search outside a git repository walks the tree', async () => {
  const root = project({ 'src/app.ts': 'a', 'biblio/app.md': 'b' })
  const result = await searchProject(root, 'app', gitIn(root))
  assert.deepEqual(result.paths, [{ path: 'src/app.ts', type: 'file' }])
})

test('read kinds', () => {
  const root = project({ 'a.txt': 'hello', 'b.bin': 'a\0b', 'c.png': 'x' })
  assert.deepEqual(readProjectFile(root, 'a.txt'), { kind: 'text', text: 'hello' })
  assert.deepEqual(readProjectFile(root, 'b.bin'), { kind: 'binary' })
  assert.equal(readProjectFile(root, 'c.png')?.kind, 'image')
  assert.equal(imagePath(root, 'c.png'), join(root, 'c.png'))
  assert.equal(imagePath(root, 'a.txt'), null)
  writeFileSync(join(root, 'big.txt'), 'x'.repeat(2 * 1024 * 1024 + 1))
  assert.deepEqual(readProjectFile(root, 'big.txt'), { kind: 'tooLarge' })
})

test('create, rename, move, copy and duplicate stay inside the root', () => {
  const root = project({ 'src/a.ts': 'a', 'lib/a.ts': 'other' })
  assert.deepEqual(createFile(root, 'src', 'b.ts'), { ok: true, path: 'src/b.ts' })
  assert.deepEqual(createFile(root, 'src', 'b.ts'), { error: 'exists' })
  assert.deepEqual(createFolder(root, '', 'biblio'), { error: 'invalidName' })
  assert.deepEqual(createFolder(root, '', 'docs'), { ok: true, path: 'docs' })
  assert.deepEqual(createFile(root, '../', 'x'), { error: 'outside' })

  assert.deepEqual(renameEntry(root, 'src/b.ts', 'c.ts'), { ok: true, path: 'src/c.ts' })
  assert.deepEqual(renameEntry(root, 'src/c.ts', 'a.ts'), { error: 'exists' })
  assert.deepEqual(renameEntry(root, 'src/c.ts', '../c.ts'), { error: 'invalidName' })

  assert.deepEqual(moveEntries(root, ['src/a.ts'], 'lib'), { error: 'exists', done: [] })
  assert.deepEqual(moveEntries(root, ['src/c.ts'], 'docs'), { ok: true, paths: ['docs/c.ts'] })
  assert.deepEqual(moveEntries(root, ['docs'], 'docs'), { error: 'intoItself', done: [] })
  assert.deepEqual(moveEntries(root, ['lib/a.ts'], 'biblio'), { error: 'outside', done: [] })

  assert.deepEqual(copyEntries(root, ['src/a.ts'], 'lib'), { ok: true, paths: ['lib/a copy.ts'] })
  assert.equal(readFileSync(join(root, 'lib', 'a copy.ts'), 'utf-8'), 'a')
  assert.deepEqual(copyEntries(root, ['src'], 'src'), { error: 'intoItself', done: [] })
  assert.deepEqual(duplicateEntry(root, 'src'), { ok: true, path: 'src copy' })
  assert.ok(existsSync(join(root, 'src copy', 'a.ts')))
  assert.deepEqual(duplicateEntry(root, 'src/a.ts'), { ok: true, path: 'src/a copy.ts' })
})
