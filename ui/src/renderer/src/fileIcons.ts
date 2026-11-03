// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { IconShape } from './tabKinds'

// The explorer's icons, drawn in the app's own 14 by 14 stroke style. A file is a page with a
// glyph for its family, a folder is the plain folder shape, and the color comes from a
// --color-icon-* token in style.css so the palette stays in one place.

export type FileFamily =
  | 'ts'
  | 'js'
  | 'go'
  | 'python'
  | 'rust'
  | 'component'
  | 'code'
  | 'shell'
  | 'markup'
  | 'style'
  | 'config'
  | 'data'
  | 'image'
  | 'archive'
  | 'doc'
  | 'lock'
  | 'file'

export type FolderFamily =
  'folder' | 'src' | 'test' | 'deps' | 'build' | 'assets' | 'docs' | 'tooling' | 'scripts'

export interface FileIcon {
  shapes: IconShape[]
  color: string
}

const EXTENSIONS: Record<string, FileFamily> = {}

function family(name: FileFamily, extensions: string[]): void {
  for (const ext of extensions) EXTENSIONS[ext] = name
}

family('ts', ['ts', 'tsx', 'mts', 'cts'])
family('js', ['js', 'jsx', 'mjs', 'cjs'])
family('go', ['go'])
family('python', ['py', 'pyi', 'pyw', 'ipynb'])
family('rust', ['rs'])
family('component', ['vue', 'svelte', 'astro'])
family('code', [
  'c',
  'h',
  'cc',
  'cpp',
  'cxx',
  'hpp',
  'hh',
  'cs',
  'java',
  'kt',
  'kts',
  'scala',
  'swift',
  'm',
  'mm',
  'rb',
  'php',
  'lua',
  'dart',
  'ex',
  'exs',
  'erl',
  'hs',
  'clj',
  'zig',
  'nim',
  'r',
  'jl',
  'pl',
  'groovy',
  'fs',
  'ml',
  'elm',
  'sol',
  'wasm',
  'wat'
])
family('shell', ['sh', 'bash', 'zsh', 'fish', 'ps1', 'psm1', 'bat', 'cmd'])
family('markup', ['html', 'htm', 'xml', 'xhtml', 'xsl', 'svg', 'vtt', 'hbs', 'ejs', 'njk', 'pug'])
family('style', ['css', 'scss', 'sass', 'less', 'styl', 'pcss'])
family('config', [
  'yaml',
  'yml',
  'toml',
  'ini',
  'cfg',
  'conf',
  'env',
  'properties',
  'editorconfig',
  'service',
  'plist'
])
family('data', [
  'json',
  'jsonc',
  'json5',
  'csv',
  'tsv',
  'sql',
  'db',
  'sqlite',
  'graphql',
  'gql',
  'proto',
  'parquet',
  'ndjson'
])
family('image', ['png', 'jpg', 'jpeg', 'gif', 'webp', 'bmp', 'ico', 'icns', 'tiff', 'avif'])
family('archive', [
  'zip',
  'tar',
  'gz',
  'tgz',
  'bz2',
  'xz',
  'zst',
  '7z',
  'rar',
  'jar',
  'rpm',
  'deb',
  'dmg',
  'apk'
])
family('doc', ['md', 'mdx', 'txt', 'rst', 'adoc', 'pdf', 'rtf', 'tex', 'org'])
family('lock', ['lock', 'lockb'])

// Whole file names that say more than their extension.
const NAMES: Record<string, FileFamily> = {
  dockerfile: 'config',
  containerfile: 'config',
  makefile: 'config',
  gnumakefile: 'config',
  justfile: 'config',
  procfile: 'config',
  '.gitignore': 'config',
  '.gitattributes': 'config',
  '.gitmodules': 'config',
  '.dockerignore': 'config',
  '.npmrc': 'config',
  '.nvmrc': 'config',
  '.prettierrc': 'config',
  '.eslintrc': 'config',
  '.env': 'config',
  'go.mod': 'config',
  'go.sum': 'lock',
  'package-lock.json': 'lock',
  'pnpm-lock.yaml': 'lock',
  'npm-shrinkwrap.json': 'lock',
  'cargo.lock': 'lock',
  license: 'doc',
  licence: 'doc',
  copying: 'doc',
  notice: 'doc',
  readme: 'doc',
  changelog: 'doc',
  authors: 'doc'
}

const FOLDERS: Record<string, FolderFamily> = {
  src: 'src',
  lib: 'src',
  app: 'src',
  pkg: 'src',
  internal: 'src',
  cmd: 'src',
  test: 'test',
  tests: 'test',
  __tests__: 'test',
  spec: 'test',
  specs: 'test',
  e2e: 'test',
  testdata: 'test',
  fixtures: 'test',
  integration_test: 'test',
  node_modules: 'deps',
  vendor: 'deps',
  bower_components: 'deps',
  '.venv': 'deps',
  venv: 'deps',
  dist: 'build',
  build: 'build',
  out: 'build',
  target: 'build',
  release: 'build',
  coverage: 'build',
  '.next': 'build',
  '.nuxt': 'build',
  assets: 'assets',
  public: 'assets',
  static: 'assets',
  images: 'assets',
  img: 'assets',
  icons: 'assets',
  media: 'assets',
  fonts: 'assets',
  resources: 'assets',
  docs: 'docs',
  doc: 'docs',
  documentation: 'docs',
  '.github': 'tooling',
  '.gitlab': 'tooling',
  '.vscode': 'tooling',
  '.idea': 'tooling',
  '.config': 'tooling',
  '.husky': 'tooling',
  '.devcontainer': 'tooling',
  '.circleci': 'tooling',
  scripts: 'scripts',
  script: 'scripts',
  bin: 'scripts',
  tools: 'scripts'
}

export function fileFamilyOf(name: string): FileFamily {
  const lower = name.toLowerCase()
  const named = NAMES[lower] ?? NAMES[lower.replace(/\.[^.]*$/, '')]
  if (named) return named
  if (lower.startsWith('.env.')) return 'config'
  if (lower.endsWith('.lock') || lower.endsWith('-lock.json') || lower.endsWith('.lock.json')) {
    return 'lock'
  }
  if (/^(tsconfig|jsconfig)(\..*)?\.json$/.test(lower)) return 'config'
  if (/\.config\.[cm]?[jt]s$/.test(lower) || /rc\.[cm]?js$/.test(lower)) return 'config'
  if (lower.endsWith('.d.ts')) return 'ts'
  const dot = lower.lastIndexOf('.')
  if (dot <= 0) return 'file'
  return EXTENSIONS[lower.slice(dot + 1)] ?? 'file'
}

export function folderFamilyOf(name: string): FolderFamily {
  return FOLDERS[name.toLowerCase()] ?? 'folder'
}

const path = (d: string): IconShape => ({ tag: 'path', attrs: { d } })

const PAGE: IconShape[] = [path('M3 1.5h5l3 3v8H3z'), path('M8 1.5v3h3')]

const CODE = path('M6 7.5 4.75 9 6 10.5M8 7.5 9.25 9 8 10.5')

const GLYPHS: Record<FileFamily, IconShape[]> = {
  ts: [CODE],
  js: [CODE],
  go: [CODE],
  python: [CODE],
  rust: [CODE],
  component: [CODE],
  code: [CODE],
  shell: [path('M4.75 7.5 6.25 9 4.75 10.5M7.25 10.5h2.5')],
  markup: [path('M5.5 7.5 4.25 9 5.5 10.5M8.5 7.5 9.75 9 8.5 10.5M7.6 7.1 6.4 10.9')],
  style: [path('M6 7v4.2M8 7v4.2M4.8 8.4h4.4M4.8 9.8h4.4')],
  config: [
    path('M4.5 7.5h5M4.5 10.2h5'),
    { tag: 'circle', attrs: { cx: 6, cy: 7.5, r: 0.9 } },
    { tag: 'circle', attrs: { cx: 8, cy: 10.2, r: 0.9 } }
  ],
  data: [
    path(
      'M5.6 7c-.7 0-.8.5-.8 1s-.3.85-.7.85c.4 0 .7.35.7.85s.1 1 .8 1M8.4 7c.7 0 .8.5.8 1s.3.85.7.85c-.4 0-.7.35-.7.85s-.1 1-.8 1'
    )
  ],
  image: [],
  archive: [
    path('M7 5.5v.8M7 7.3v.8'),
    { tag: 'rect', attrs: { x: 6, y: 9, width: 2, height: 2.2, rx: 0.4 } }
  ],
  doc: [path('M5 7.5h4M5 9.2h4M5 10.9h2.6')],
  lock: [
    { tag: 'rect', attrs: { x: 4.8, y: 8.6, width: 4.4, height: 2.9, rx: 0.5 } },
    path('M5.8 8.6V7.7a1.2 1.2 0 0 1 2.4 0v.9')
  ],
  file: []
}

const IMAGE: IconShape[] = [
  { tag: 'rect', attrs: { x: 1.5, y: 2.5, width: 11, height: 9, rx: 1 } },
  { tag: 'circle', attrs: { cx: 5, cy: 5.5, r: 1 } },
  path('M1.5 10l3.5-3 2.5 2 2-1.5 3 2.5')
]

const FOLDER_CLOSED: IconShape[] = [path('M1.5 3.5h4l1.5 2h5.5v6h-11z')]
const FOLDER_OPEN: IconShape[] = [
  path('M1.5 11.5v-8h4l1.5 2h4.5v1.5'),
  path('M1.5 11.5 3.3 7h9.2l-1.8 4.5z')
]

export function fileIconOf(name: string): FileIcon {
  const fam = fileFamilyOf(name)
  const shapes = fam === 'image' ? IMAGE : [...PAGE, ...GLYPHS[fam]]
  return { shapes, color: `var(--color-icon-${fam})` }
}

export function folderIconOf(name: string, open: boolean): FileIcon {
  const fam = folderFamilyOf(name)
  return {
    shapes: open ? FOLDER_OPEN : FOLDER_CLOSED,
    color: fam === 'folder' ? 'var(--color-icon-folder)' : `var(--color-icon-folder-${fam})`
  }
}
