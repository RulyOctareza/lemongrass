// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { computed, nextTick, ref, shallowRef, watch, type Ref } from 'vue'
import type { AppInputHandle } from './components/appInput'
import {
  buildSearchTree,
  normalizeQuery,
  type SearchCandidate,
  type SearchResult,
  type SearchRow
} from './treeSearch'

export interface CandidateSet {
  items: SearchCandidate[]
  // The source stopped early, so better matches may exist beyond what is shown.
  truncated?: boolean
}

export interface TreeSearchOptions {
  candidates: (query: string) => CandidateSet | Promise<CandidateSet>
  open: (row: SearchRow) => void
  // Where focus returns when the search closes, the panel itself.
  panel: Ref<HTMLElement | null>
  debounceMs?: number
}

export interface TreeSearch {
  open: Ref<boolean>
  query: Ref<string>
  result: Ref<SearchResult | null>
  truncated: Ref<boolean>
  cursor: Ref<number>
  input: Ref<AppInputHandle | null>
  active: Ref<boolean>
  onPanelKeydown: (event: KeyboardEvent) => void
  onInputKeydown: (event: KeyboardEvent) => void
  onInputBlur: () => void
  close: () => void
  refresh: () => void
  setQuery: (text: string) => void
  pick: (index: number) => void
  bindInput: (handle: unknown) => void
}

function typedIntoField(event: KeyboardEvent): boolean {
  const target = event.target as HTMLElement | null
  return !!target?.closest('input, textarea, [contenteditable="true"]')
}

// The sidebar's type-to-search, one piece for the Biblio and Files views: a printable key while
// the panel has focus opens the query field with that key in it, the panel then shows the ranked
// matches as a tree, arrows move, Enter opens and Escape clears.
export function useTreeSearch(options: TreeSearchOptions): TreeSearch {
  const open = ref(false)
  const query = ref('')
  const result = shallowRef<SearchResult | null>(null)
  const truncated = ref(false)
  const cursor = ref(0)
  const input = ref<AppInputHandle | null>(null)
  const active = computed(() => open.value && normalizeQuery(query.value) !== '')

  let sequence = 0
  let timer: ReturnType<typeof setTimeout> | undefined

  async function run(): Promise<void> {
    const mine = ++sequence
    const text = query.value
    if (!normalizeQuery(text)) {
      result.value = null
      truncated.value = false
      return
    }
    const set = await options.candidates(text)
    // A slower reply for an older query never replaces a newer one.
    if (mine !== sequence) return
    result.value = buildSearchTree(set.items, text)
    truncated.value = !!set.truncated
    cursor.value = Math.min(cursor.value, Math.max(0, result.value.rows.length - 1))
  }

  function schedule(): void {
    clearTimeout(timer)
    cursor.value = 0
    timer = setTimeout(() => void run(), options.debounceMs ?? 0)
  }

  watch(query, schedule)

  function refresh(): void {
    if (active.value) void run()
  }

  function close(): void {
    clearTimeout(timer)
    sequence++
    open.value = false
    query.value = ''
    result.value = null
    cursor.value = 0
    options.panel.value?.focus()
  }

  function onPanelKeydown(event: KeyboardEvent): void {
    if (event.defaultPrevented || typedIntoField(event)) return
    if (event.key.length !== 1 || event.ctrlKey || event.metaKey || event.altKey) return
    if (event.key === ' ' && !open.value) return
    event.preventDefault()
    open.value = true
    query.value += event.key
    void nextTick(() => input.value?.focus())
  }

  function move(delta: number): void {
    const rows = result.value?.rows.length ?? 0
    if (!rows) return
    cursor.value = (cursor.value + delta + rows) % rows
  }

  function onInputKeydown(event: KeyboardEvent): void {
    if (event.key === 'ArrowDown') move(1)
    else if (event.key === 'ArrowUp') move(-1)
    else if (event.key === 'Enter') {
      const row = result.value?.rows[cursor.value]
      if (row) options.open(row)
    } else if (event.key === 'Escape') close()
    else return
    event.preventDefault()
    event.stopPropagation()
  }

  function onInputBlur(): void {
    if (!query.value) open.value = false
  }

  function setQuery(text: string): void {
    query.value = text
  }

  function pick(index: number): void {
    const row = result.value?.rows[index]
    if (!row) return
    cursor.value = index
    options.open(row)
  }

  function bindInput(handle: unknown): void {
    input.value = (handle as AppInputHandle | null) ?? null
  }

  return {
    open,
    query,
    result,
    truncated,
    cursor,
    input,
    active,
    onPanelKeydown,
    onInputKeydown,
    onInputBlur,
    close,
    refresh,
    setQuery,
    pick,
    bindInput
  }
}
