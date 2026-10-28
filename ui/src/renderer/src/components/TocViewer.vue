<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { openFile, type ProjectRef } from '../workspace'
import type { BiblioNode } from '../../../preload/types'

const props = defineProps<{
  project: ProjectRef
}>()

interface ChapterEntry {
  path: string
  title: string
  updated?: number
}

interface WarningPart {
  text: string
  book?: string
  code?: boolean
}

interface BookEntry {
  kind: 'book'
  name: string
  title: string
  written?: number
  updated?: number
  tags: string[]
  warning: WarningPart[]
  chapters: ChapterEntry[]
}

interface TextEntry {
  kind: 'text'
  text: string
}

type TocEntry = BookEntry | TextEntry

const BOOK_LINE = /^([^\s[]+)\s+\[([^\]]*)\](?:\s+warning:\s*(.+))?$/
// The bracket form, books `title[date][tags]` and chapters `title[tags].md`, is still read for projects that use it.
const LEGACY_BOOK_LINE = /^([^\s[]+)\[\d{4}-\d{2}-\d{2}\]\[([^\]]*)\](?:\s+warning:\s*(.+))?$/
const LEGACY_TAGS = /\[[^\]]*\]$/

function humanize(snakeCase: string): string {
  const spaced = snakeCase.replaceAll('_', ' ')
  return spaced.charAt(0).toUpperCase() + spaced.slice(1)
}

function splitTags(raw: string): string[] {
  return raw
    .split(',')
    .map((t) => t.trim())
    .filter(Boolean)
}

function parseChapter(node: BiblioNode): ChapterEntry {
  const name = node.name.replace(/\.md$/, '').replace(LEGACY_TAGS, '')
  return { path: node.path, title: humanize(name), updated: node.modifiedMs }
}

const DATE_FORMAT = new Intl.DateTimeFormat('en-GB', {
  day: 'numeric',
  month: 'short',
  year: 'numeric'
})

function formatDate(ms: number): string {
  return DATE_FORMAT.format(new Date(ms))
}

// A book's folder mtime moves only when entries are added or removed, so its newest chapter says when it was last updated.
function lastUpdated(
  bookDir: BiblioNode | undefined,
  chapters: ChapterEntry[]
): number | undefined {
  const times = chapters.map((c) => c.updated).filter((t): t is number => t !== undefined)
  return times.length ? Math.max(...times) : bookDir?.modifiedMs
}

function matchBookLine(line: string): RegExpExecArray | null {
  return BOOK_LINE.exec(line) ?? LEGACY_BOOK_LINE.exec(line)
}

function parseWarning(raw: string | undefined, bookNames: Set<string>): WarningPart[] {
  if (!raw) return []
  return raw
    .split(/(`[^`]+`)/)
    .filter(Boolean)
    .map((piece) => {
      const quoted = /^`([^`]+)`$/.exec(piece)
      if (!quoted) return { text: piece }
      return bookNames.has(quoted[1])
        ? { text: quoted[1], book: quoted[1] }
        : { text: quoted[1], code: true }
    })
}

function findBooksDir(tree: BiblioNode[]): BiblioNode | undefined {
  return tree.find((n) => n.type === 'dir' && n.name === 'books')
}

function findBookDir(booksDir: BiblioNode | undefined, name: string): BiblioNode | undefined {
  const dirs = booksDir?.children?.filter((n) => n.type === 'dir') ?? []
  return dirs.find((n) => n.name === name) ?? dirs.find((n) => n.name.startsWith(`${name}[`))
}

function parseToc(raw: string, tree: BiblioNode[]): TocEntry[] {
  const booksDir = findBooksDir(tree)
  const lines = raw
    .split('\n')
    .map((line) => line.trim())
    .filter((line) => line.length > 0)
  const bookNames = new Set(
    lines.map((line) => matchBookLine(line)?.[1]).filter((n): n is string => !!n)
  )
  return lines.map((line): TocEntry => {
    const bookMatch = matchBookLine(line)
    if (!bookMatch) return { kind: 'text', text: line }
    const [, name, tagsRaw, warningRaw] = bookMatch
    const bookDir = findBookDir(booksDir, name)
    const chapters = (bookDir?.children ?? [])
      .filter((n) => n.type === 'file' && n.name.endsWith('.md'))
      .map(parseChapter)
    return {
      kind: 'book',
      name,
      title: humanize(name),
      written: bookDir?.createdMs,
      updated: lastUpdated(bookDir, chapters),
      tags: splitTags(tagsRaw),
      warning: parseWarning(warningRaw, bookNames),
      chapters
    }
  })
}

const entries = ref<TocEntry[]>([])
const loading = ref(true)

async function load(): Promise<void> {
  loading.value = true
  const [raw, tree] = await Promise.all([
    window.api.biblio.read(props.project.path, 'books/toc.md'),
    window.api.biblio.tree(props.project.path)
  ])
  entries.value = raw ? parseToc(raw, tree?.children ?? []) : []
  loading.value = false
}

let unsubscribe: (() => void) | undefined

onMounted(() => {
  void load()
  unsubscribe = window.api.biblio.onChanged((projectPath) => {
    if (projectPath === props.project.path) void load()
  })
})
onBeforeUnmount(() => unsubscribe?.())
watch(() => props.project.path, load)

const root = ref<HTMLElement | null>(null)

function showBook(name: string): void {
  root.value?.querySelector(`[data-book="${name}"]`)?.scrollIntoView({
    behavior: 'smooth',
    block: 'start'
  })
}

function openChapter(path: string): void {
  openFile(props.project, path)
}
</script>

<template>
  <div ref="root" class="toc-viewer">
    <p v-if="loading" class="empty">Loading...</p>
    <p v-else-if="entries.length === 0" class="empty">Nothing in books/ yet.</p>
    <template v-else>
      <template v-for="(entry, i) in entries" :key="i">
        <p v-if="entry.kind === 'text'" class="text-line">{{ entry.text }}</p>
        <div v-else class="book-card" :data-book="entry.name">
          <div class="book-header">
            <h4 class="book-title">{{ entry.title }}</h4>
            <div class="date-row">
              <span v-if="entry.written" class="date-capsule"
                >Written {{ formatDate(entry.written) }}</span
              >
              <span v-if="entry.updated" class="date-capsule"
                >Updated {{ formatDate(entry.updated) }}</span
              >
            </div>
          </div>
          <div v-if="entry.tags.length" class="tag-row">
            <span v-for="tag in entry.tags" :key="tag" class="tag-capsule">{{ tag }}</span>
          </div>
          <p v-if="entry.warning.length" class="book-warning">
            <span class="warning-label">Warning</span>
            <template v-for="(part, j) in entry.warning" :key="j">
              <button v-if="part.book" class="warning-link" @click="showBook(part.book)">
                {{ part.text }}
              </button>
              <code v-else-if="part.code">{{ part.text }}</code>
              <template v-else>{{ part.text }}</template>
            </template>
          </p>
          <div v-if="entry.chapters.length" class="chapters">
            <button
              v-for="chapter in entry.chapters"
              :key="chapter.path"
              class="chapter-row"
              @click="openChapter(chapter.path)"
            >
              <span class="chapter-title">{{ chapter.title }}</span>
              <span v-if="chapter.updated" class="chapter-date" title="Last updated">{{
                formatDate(chapter.updated)
              }}</span>
            </button>
          </div>
        </div>
      </template>
    </template>
  </div>
</template>

<style scoped>
.toc-viewer {
  flex: 1;
  min-width: 0;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-6);
  display: flex;
  flex-direction: column;
  gap: var(--space-4);
}

.empty {
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.text-line {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.book-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-4);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-xl);
}

.book-title {
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: var(--weight-semibold);
  color: var(--color-fg-primary);
}

.book-header {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
}

.date-row {
  display: flex;
  flex-shrink: 0;
  gap: var(--space-1);
}

.date-capsule {
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-surface-3);
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.tag-row {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-1);
}

.tag-capsule {
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
  font-size: var(--text-xs);
}

.book-warning {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-normal);
}

.warning-label {
  display: inline-block;
  margin-right: var(--space-2);
  padding: 1px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-error-muted);
  border: 1px solid var(--color-error);
  color: var(--color-error);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  text-transform: uppercase;
  letter-spacing: var(--tracking-wide);
}

.book-warning code {
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.warning-link {
  padding: 0;
  background: none;
  border: none;
  color: var(--color-fg-accent);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  cursor: pointer;
  text-decoration: underline;
}

.chapters {
  display: flex;
  flex-direction: column;
  gap: 2px;
  margin-top: var(--space-2);
}

.chapter-row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.chapter-row:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.chapter-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.chapter-date {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}
</style>
