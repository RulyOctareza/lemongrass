<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, provide, reactive, ref, watch } from 'vue'
import AppSegmented from '../AppSegmented.vue'
import JsonNode from './JsonNode.vue'
import HistoryPopover from './HistoryPopover.vue'
import { statusClass } from '../../resterClientCall'
import { pathId, runSearch } from '../../responseSearch'
import { treeSearchKey, type TreeSearch } from './searchContext'
import type {
  ResterClientDownload,
  ResterClientResult,
  ResterClientRun
} from '../../../../preload/types'
import AppInput from '../AppInput.vue'

const props = defineProps<{
  result: ResterClientResult | null
  download: ResterClientDownload | null
  error: string
  sending: boolean
  // Set when the response comes from history and its body was cut when it was stored.
  bodyCut: boolean
  // When the shown response came back, null before the first send.
  at: string | null
  runs: ResterClientRun[]
  runId: string | null
}>()

const emit = defineEmits<{
  pickRun: [run: ResterClientRun]
  removeRun: [runId: string]
  clearHistory: []
}>()

const view = ref<'tree' | 'raw' | 'headers'>('tree')

const parsed = computed<{ ok: boolean; value: unknown }>(() => {
  const body = props.result?.body ?? ''
  if (!body.trim()) return { ok: false, value: null }
  try {
    return { ok: true, value: JSON.parse(body) }
  } catch {
    return { ok: false, value: null }
  }
})

const views = computed(() =>
  parsed.value.ok
    ? [
        { value: 'tree' as const, label: 'Tree' },
        { value: 'raw' as const, label: 'Raw' },
        { value: 'headers' as const, label: 'Headers' }
      ]
    : [
        { value: 'raw' as const, label: 'Body' },
        { value: 'headers' as const, label: 'Headers' }
      ]
)

const activeView = computed(() => (view.value === 'tree' && !parsed.value.ok ? 'raw' : view.value))

const rawText = computed(() =>
  parsed.value.ok ? JSON.stringify(parsed.value.value, null, 2) : (props.result?.body ?? '')
)

const headerRows = computed(() =>
  Object.entries(props.result?.headers ?? {}).flatMap(([name, values]) =>
    values.map((value) => ({ name, value }))
  )
)

// A large body takes a moment to walk, so the search waits for typing to pause.
const SEARCH_DELAY_MS = 150

const query = ref('')
const appliedQuery = ref('')
const currentIndex = ref(0)
let searchTimer: ReturnType<typeof setTimeout> | null = null

const filterOptions = [
  { value: 'all' as const, label: 'All' },
  { value: 'matches' as const, label: 'Matches' }
]

const tree = reactive<TreeSearch>({
  active: false,
  byPath: new Map(),
  ancestors: new Set(),
  records: new Set(),
  currentId: null,
  filter: false,
  version: 0
})
provide(treeSearchKey, tree)

const searchOutcome = computed(() =>
  parsed.value.ok ? runSearch(parsed.value.value, appliedQuery.value) : null
)
const searchResult = computed(() =>
  searchOutcome.value && 'result' in searchOutcome.value ? searchOutcome.value.result : null
)
const searchError = computed(() =>
  searchOutcome.value && 'error' in searchOutcome.value ? searchOutcome.value.error : ''
)

const stepCount = computed(() => searchResult.value?.steps.length ?? 0)

const countText = computed(() => {
  const result = searchResult.value
  if (!result) return searchError.value
  const records = result.unit === 'record'
  if (stepCount.value === 0) return records ? 'No record' : 'No match'
  const noun = records ? (stepCount.value === 1 ? ' record' : ' records') : ''
  return `${currentIndex.value + 1} of ${stepCount.value}${noun}`
})

watch(query, (text) => {
  if (searchTimer) clearTimeout(searchTimer)
  searchTimer = setTimeout(() => (appliedQuery.value = text), SEARCH_DELAY_MS)
  if (text.trim() && parsed.value.ok) view.value = 'tree'
})

watch(searchResult, (result) => {
  currentIndex.value = 0
  tree.active = !!result
  tree.byPath = result?.byPath ?? new Map()
  tree.ancestors = result?.ancestors ?? new Set()
  tree.records = result?.records ?? new Set()
  tree.version++
  updateCurrent()
})

function updateCurrent(): void {
  const step = searchResult.value?.steps[currentIndex.value]
  tree.currentId = step ? pathId(step) : null
}

function step(delta: number): void {
  if (stepCount.value === 0) return
  currentIndex.value = (currentIndex.value + delta + stepCount.value) % stepCount.value
  updateCurrent()
}

function onSearchKeydown(event: KeyboardEvent): void {
  if (event.key === 'Enter') {
    event.preventDefault()
    step(event.shiftKey ? -1 : 1)
  } else if (event.key === 'Escape') {
    query.value = ''
    appliedQuery.value = ''
  }
}

function formatSize(bytes: number): string {
  if (bytes < 1024) return `${bytes} B`
  if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`
  return `${(bytes / 1024 / 1024).toFixed(1)} MB`
}
</script>

<template>
  <section class="box">
    <header class="box-head">
      <span class="box-title">Response</span>
      <template v-if="!sending && !error && result">
        <span class="status" :class="statusClass(result.status)">{{ result.status }}</span>
        <span class="meta">{{ result.durationMs }} ms</span>
        <span class="meta">{{ formatSize(result.size) }}</span>
        <span class="spacer" />
        <AppSegmented
          class="views"
          :model-value="activeView"
          :options="views"
          aria-label="Response view"
          @update:model-value="view = $event"
        />
      </template>
      <template v-else-if="!sending && !error && download?.kind === 'saved'">
        <span class="status" :class="statusClass(download.status)">{{ download.status }}</span>
        <span class="meta">{{ download.durationMs }} ms</span>
        <span class="meta">{{ formatSize(download.bytes) }}</span>
        <span class="spacer" />
      </template>
      <span v-else class="spacer" />
      <HistoryPopover
        v-if="at && !sending"
        :at="at"
        :runs="runs"
        :run-id="runId"
        @pick="emit('pickRun', $event)"
        @remove="emit('removeRun', $event)"
        @clear="emit('clearHistory')"
      />
    </header>
    <div v-if="!sending && !error && result && parsed.ok" class="search-row">
      <AppInput
        v-model="query"
        size="sm"
        mono
        class="search-input"
        type="text"
        spellcheck="false"
        placeholder="Search, key:value, a.b.*.c, price>100, AND OR NOT"
        title="Plain text matches keys and values. key:value matches under a key. A dotted path picks fields, * for any key or index. Compare with > >= < <= = !=. Quotes force plain text. Chain terms with AND, OR, NOT and parentheses to find records, objects in a list, where all of it holds."
        @keydown="onSearchKeydown"
      />
      <span v-if="countText" class="count" :class="{ invalid: searchError }">
        {{ countText }}
      </span>
      <button
        type="button"
        class="step"
        title="Previous match (Shift+Enter)"
        :disabled="stepCount === 0"
        @click="step(-1)"
      >
        ↑
      </button>
      <button
        type="button"
        class="step"
        title="Next match (Enter)"
        :disabled="stepCount === 0"
        @click="step(1)"
      >
        ↓
      </button>
      <AppSegmented
        :model-value="tree.filter ? 'matches' : 'all'"
        :options="filterOptions"
        aria-label="Show"
        @update:model-value="tree.filter = $event === 'matches'"
      />
    </div>

    <div class="box-body">
      <p v-if="sending" class="box-placeholder">Sending...</p>
      <p v-else-if="error" class="error-text">{{ error }}</p>
      <p v-else-if="download?.kind === 'saved'" class="box-placeholder">
        Saved to {{ download.path }}
      </p>
      <p v-else-if="download?.kind === 'cancelled'" class="box-placeholder">Download cancelled.</p>

      <template v-else-if="result">
        <p v-if="result.truncated" class="hint">
          Only the first {{ formatSize(result.size) }} came back inline. Use Send and download for
          the whole body.
        </p>
        <p v-if="bodyCut" class="hint">This saved response was cut when it went into history.</p>
        <template v-if="activeView === 'headers'">
          <div v-for="(row, i) in headerRows" :key="i" class="header-row">
            <span class="header-name">{{ row.name }}</span>
            <span class="header-value">{{ row.value }}</span>
          </div>
        </template>
        <p v-else-if="result.binary" class="box-placeholder">
          The body is binary. Use Send and download to save it.
        </p>
        <p v-else-if="!result.body" class="box-placeholder">The response has no body.</p>
        <div v-else-if="activeView === 'tree'" class="tree">
          <JsonNode :value="parsed.value" :depth="0" :last="true" :path="[]" />
        </div>
        <pre v-else class="raw">{{ rawText }}</pre>
      </template>

      <p v-else class="box-placeholder">Send the request to see its response here.</p>
    </div>
  </section>
</template>

<style scoped>
.meta {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.spacer {
  flex: 1;
}

.tree {
  padding-left: var(--space-3);
}

.search-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  border-bottom: 1px solid var(--color-border-default);
}

.search-input {
  flex: 1;
  min-width: 0;
}

.count {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  white-space: nowrap;
}

.count.invalid {
  color: var(--color-error);
}

.step {
  width: 26px;
  height: 26px;
  padding: 0;
  background: transparent;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  cursor: pointer;
}

.step:hover:not(:disabled) {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}

.step:disabled {
  opacity: 0.4;
  cursor: default;
}

.raw {
  margin: 0;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: 1.6;
  white-space: pre-wrap;
  word-break: break-all;
  color: var(--color-fg-primary);
}

.header-row {
  display: grid;
  grid-template-columns: minmax(120px, 1fr) minmax(0, 2fr);
  gap: var(--space-3);
  padding: 2px 0;
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.header-name {
  color: var(--color-sapphire);
}

.header-value {
  color: var(--color-fg-primary);
  word-break: break-all;
}
</style>
