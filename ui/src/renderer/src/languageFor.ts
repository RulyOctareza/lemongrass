// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { StreamLanguage, type StreamParser } from '@codemirror/language'
import type { Extension } from '@codemirror/state'

// The built-in grammars for the read-only reader. Every grammar is a dynamic import, so none of
// them lands in the startup chunk and a language loads the first time a file of it is opened.
// Language packs are data only and cannot carry a grammar, so a pack names one of these ids.

export type LanguageId =
  | 'javascript'
  | 'jsx'
  | 'typescript'
  | 'tsx'
  | 'vue'
  | 'go'
  | 'markdown'
  | 'json'
  | 'yaml'
  | 'css'
  | 'html'
  | 'sql'
  | 'rust'
  | 'php'
  | 'python'
  | 'shell'
  | 'dockerfile'
  | 'toml'

// Whole names, lowercased, win over extensions.
const byName = new Map<string, LanguageId>([
  ['dockerfile', 'dockerfile'],
  ['containerfile', 'dockerfile']
])

const byExtension = new Map<string, LanguageId>([
  ['js', 'javascript'],
  ['mjs', 'javascript'],
  ['cjs', 'javascript'],
  ['jsx', 'jsx'],
  ['ts', 'typescript'],
  ['mts', 'typescript'],
  ['cts', 'typescript'],
  ['tsx', 'tsx'],
  ['vue', 'vue'],
  ['go', 'go'],
  ['md', 'markdown'],
  ['markdown', 'markdown'],
  ['json', 'json'],
  ['yml', 'yaml'],
  ['yaml', 'yaml'],
  ['css', 'css'],
  ['html', 'html'],
  ['htm', 'html'],
  ['sql', 'sql'],
  ['rs', 'rust'],
  ['php', 'php'],
  ['py', 'python'],
  ['pyi', 'python'],
  ['sh', 'shell'],
  ['bash', 'shell'],
  ['zsh', 'shell'],
  ['dockerfile', 'dockerfile'],
  ['containerfile', 'dockerfile'],
  ['toml', 'toml']
])

// The language of a path with `/` separators, or null for plain text. `Dockerfile.dev` and the
// like count as Dockerfiles; a dotfile such as `.bashrc` has no extension.
export function languageIdFor(path: string): LanguageId | null {
  const name = path.slice(path.lastIndexOf('/') + 1).toLowerCase()
  const named = byName.get(name)
  if (named) return named
  if (name.startsWith('dockerfile.') || name.startsWith('containerfile.')) return 'dockerfile'
  const dot = name.lastIndexOf('.')
  if (dot <= 0) return null
  return byExtension.get(name.slice(dot + 1)) ?? null
}

// @codemirror/language is in the startup chunk already, through the conflict resolver, so only
// the parser itself is imported on demand.
async function legacy(load: () => Promise<StreamParser<unknown>>): Promise<Extension> {
  return StreamLanguage.define(await load())
}

const loaders: Record<LanguageId, () => Promise<Extension>> = {
  javascript: async () => (await import('@codemirror/lang-javascript')).javascript(),
  jsx: async () => (await import('@codemirror/lang-javascript')).javascript({ jsx: true }),
  typescript: async () =>
    (await import('@codemirror/lang-javascript')).javascript({ typescript: true }),
  tsx: async () =>
    (await import('@codemirror/lang-javascript')).javascript({ jsx: true, typescript: true }),
  vue: async () => (await import('@codemirror/lang-vue')).vue(),
  go: async () => (await import('@codemirror/lang-go')).go(),
  markdown: async () => (await import('@codemirror/lang-markdown')).markdown(),
  json: async () => (await import('@codemirror/lang-json')).json(),
  yaml: async () => (await import('@codemirror/lang-yaml')).yaml(),
  css: async () => (await import('@codemirror/lang-css')).css(),
  html: async () => (await import('@codemirror/lang-html')).html(),
  sql: async () => (await import('@codemirror/lang-sql')).sql(),
  rust: async () => (await import('@codemirror/lang-rust')).rust(),
  php: async () => (await import('@codemirror/lang-php')).php(),
  python: async () => (await import('@codemirror/lang-python')).python(),
  shell: () => legacy(async () => (await import('@codemirror/legacy-modes/mode/shell')).shell),
  dockerfile: () =>
    legacy(async () => (await import('@codemirror/legacy-modes/mode/dockerfile')).dockerFile),
  toml: () => legacy(async () => (await import('@codemirror/legacy-modes/mode/toml')).toml)
}

const loaded = new Map<LanguageId, Promise<Extension>>()

// The grammar for a path, or null for plain text. A failed import is dropped from the cache so
// the next file of that language tries again.
export async function loadLanguage(path: string): Promise<Extension | null> {
  const id = languageIdFor(path)
  if (!id) return null
  let pending = loaded.get(id)
  if (!pending) {
    pending = loaders[id]()
    loaded.set(id, pending)
    pending.catch(() => loaded.delete(id))
  }
  return pending
}
