<script setup lang="ts">
import { computed, onBeforeUnmount, ref, watch } from 'vue'
import { gitStateOf, pushBranch, refreshGit } from '../git'
import { formatAge, formatDate } from '../gitTime'
import { buildGraph } from '../historyGraph'
import type { ProjectRef } from '../workspace'
import type { GitCommit, GitCommitInfo, GitLogSearchMode } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppContextMenu, { type MenuItem } from './AppContextMenu.vue'
import AppSegmented from './AppSegmented.vue'
import HistoryAmendModal from './HistoryAmendModal.vue'
import GitIcon from './GitIcon.vue'
import HistoryResetModal from './HistoryResetModal.vue'
import RevisionView from './RevisionView.vue'

const PAGE = 200
const LANE_WIDTH = 14
const ROW_HEIGHT = 26
const MAX_LANES = 8
const MAX_REFS = 2
const LANE_COLORS = [
  'var(--color-info)',
  'var(--color-success)',
  'var(--color-amber)',
  'var(--color-error)',
  'var(--color-fg-secondary)'
]

const props = defineProps<{
  project: ProjectRef
}>()

const SEARCH_MODES: { value: GitLogSearchMode; label: string }[] = [
  { value: 'message', label: 'Message' },
  { value: 'author', label: 'Author' },
  { value: 'hash', label: 'Hash' },
  { value: 'file', label: 'File' }
]

const state = computed(() => gitStateOf(props.project.id))
const commits = ref<GitCommit[]>([])
const hasMore = ref(false)
const head = ref<string | null>(null)
const loading = ref(true)
const error = ref<string | null>(null)
const search = ref('')
const mode = ref<GitLogSearchMode>('message')
const selected = ref<string | null>(null)
const info = ref<GitCommitInfo | null>(null)
const detailMode = ref<'diff' | 'blame'>('diff')
const menu = ref<{ x: number; y: number; commit: GitCommit } | null>(null)
const resetting = ref<GitCommit | null>(null)
const amending = ref<GitCommit | null>(null)
const pushError = ref<string | null>(null)

const status = computed(() => state.value.status)
// Without an upstream the ahead count is unknown, and a push publishes the branch.
const pushBlockedReason = computed((): string | null => {
  if (state.value.busy) return 'An operation is running'
  if (status.value?.detached) return 'HEAD is detached'
  if (!status.value?.pushTarget) return 'No remote to push to'
  if (status.value.upstream && status.value.ahead === 0) return 'Nothing to push'
  return null
})
const pushLabel = computed(() => {
  if (state.value.busy === 'push') return 'Pushing...'
  if (status.value?.upstream && status.value.ahead > 0) return `Push ${status.value.ahead}`
  return 'Push'
})

async function push(): Promise<void> {
  pushError.value = null
  pushError.value = await pushBranch(props.project)
}

let logRequest = 0
let lastToken: string | null = null
let searchTimer: ReturnType<typeof setTimeout> | undefined

const searching = computed(() => search.value.trim().length > 0)

// A search drops commits from the middle of the history, so their lanes would not connect.
const graph = computed(() => (searching.value ? null : buildGraph(commits.value)))
const graphLanes = computed(() => Math.min(graph.value?.lanes ?? 0, MAX_LANES))
const graphWidth = computed(() => graphLanes.value * LANE_WIDTH + 6)
const selectedCommit = computed(() => commits.value.find((c) => c.hash === selected.value))

async function load(keep: number): Promise<void> {
  const id = ++logRequest
  const result = await window.api.git.log(
    props.project.path,
    mode.value,
    search.value,
    0,
    Math.max(PAGE, keep)
  )
  if (id !== logRequest) return
  loading.value = false
  if (result.status === 'error') {
    error.value = result.message
    return
  }
  error.value = null
  commits.value = result.commits
  hasMore.value = result.hasMore
  head.value = result.head
  if (selected.value && !result.commits.some((c) => c.hash === selected.value)) {
    selected.value = null
  }
}

async function loadMore(): Promise<void> {
  if (!hasMore.value || loading.value) return
  const id = ++logRequest
  loading.value = true
  const result = await window.api.git.log(
    props.project.path,
    mode.value,
    search.value,
    commits.value.length,
    PAGE
  )
  if (id !== logRequest) return
  loading.value = false
  if (result.status !== 'ok') return
  commits.value = [...commits.value, ...result.commits]
  hasMore.value = result.hasMore
}

function onScroll(event: Event): void {
  const list = event.target as HTMLElement
  if (list.scrollTop + list.clientHeight >= list.scrollHeight - 200) void loadMore()
}

// Each status poll checks one cheap token, and the log reloads only when a ref moved.
watch(
  () => state.value.revision,
  async () => {
    const token = await window.api.git.logToken(props.project.path)
    if (token === lastToken) return
    lastToken = token
    await load(commits.value.length)
  },
  { immediate: true }
)

watch([search, mode], () => {
  clearTimeout(searchTimer)
  searchTimer = setTimeout(() => {
    loading.value = true
    void load(0)
  }, 250)
})

onBeforeUnmount(() => clearTimeout(searchTimer))

watch(selected, async (hash) => {
  info.value = null
  if (!hash) return
  const next = await window.api.git.commitInfo(props.project.path, hash)
  if (selected.value === hash) info.value = next
})

function pick(commit: GitCommit): void {
  selected.value = commit.hash
}

function openMenu(event: MouseEvent, commit: GitCommit): void {
  selected.value = commit.hash
  menu.value = { x: event.clientX, y: event.clientY, commit }
}

const menuItems = computed((): MenuItem[] => {
  const commit = menu.value?.commit
  if (!commit) return []
  const isHead = commit.hash === head.value
  return [
    {
      key: 'reset',
      label: 'Reset soft to here',
      disabled: isHead,
      title: isHead ? 'This is already HEAD' : undefined
    },
    {
      key: 'amend',
      label: 'Amend commit message',
      disabled: !isHead,
      title: isHead ? undefined : 'Only the HEAD commit can be amended'
    },
    { key: 'blame', label: 'Blame' }
  ]
})

function onMenuSelect(key: string): void {
  const commit = menu.value?.commit
  menu.value = null
  if (!commit) return
  if (key === 'reset') resetting.value = commit
  else if (key === 'amend') amending.value = commit
  else if (key === 'blame') detailMode.value = 'blame'
}

async function afterRewrite(): Promise<void> {
  resetting.value = null
  amending.value = null
  await refreshGit(props.project)
}

function laneX(lane: number): number {
  return lane * LANE_WIDTH + LANE_WIDTH / 2
}
</script>

<template>
  <div class="history">
    <div class="history-bar">
      <input
        v-model="search"
        class="history-search"
        placeholder="Search commits"
        aria-label="Search commits"
      />
      <AppSegmented v-model="mode" aria-label="Search by" :options="SEARCH_MODES" />
      <AppButton
        variant="primary"
        :disabled="pushBlockedReason !== null"
        :title="pushBlockedReason ?? `Push ${status?.branch} to ${status?.pushTarget}`"
        @click="push"
      >
        {{ pushLabel }}
      </AppButton>
    </div>
    <p v-if="pushError" class="push-error">{{ pushError }}</p>

    <div class="history-body">
      <div class="history-list" @scroll="onScroll">
        <p v-if="error" class="notice error">{{ error }}</p>
        <p v-else-if="loading && commits.length === 0" class="notice">Loading...</p>
        <p v-else-if="commits.length === 0" class="notice">
          {{ searching ? 'No commits match.' : 'No commits yet.' }}
        </p>
        <button
          v-for="(commit, index) in commits"
          :key="commit.hash"
          class="commit-row"
          :class="{ selected: commit.hash === selected, head: commit.hash === head }"
          @click="pick(commit)"
          @contextmenu.prevent="openMenu($event, commit)"
        >
          <svg
            v-if="graph"
            class="commit-graph"
            :width="graphWidth"
            :height="ROW_HEIGHT"
            :viewBox="`0 0 ${graphWidth} ${ROW_HEIGHT}`"
          >
            <line
              v-for="(line, lineIndex) in graph.rows[index].lines"
              :key="lineIndex"
              :x1="laneX(line.fromLane)"
              :y1="line.fromY * ROW_HEIGHT"
              :x2="laneX(line.toLane)"
              :y2="line.toY * ROW_HEIGHT"
              :style="{ stroke: LANE_COLORS[line.color % LANE_COLORS.length] }"
              stroke-width="1.6"
              stroke-linecap="round"
            />
            <circle
              :cx="laneX(graph.rows[index].lane)"
              :cy="ROW_HEIGHT / 2"
              :r="commit.hash === head ? 4.5 : 3.5"
              :style="{
                fill:
                  commit.hash === head
                    ? 'var(--color-surface-1)'
                    : LANE_COLORS[graph.rows[index].color % LANE_COLORS.length],
                stroke: LANE_COLORS[graph.rows[index].color % LANE_COLORS.length]
              }"
              stroke-width="1.8"
            />
          </svg>
          <span class="commit-main">
            <span
              v-for="label in commit.refs.slice(0, MAX_REFS)"
              :key="`${label.kind}-${label.name}`"
              class="commit-ref"
              :class="label.kind"
              :title="label.name"
              >{{ label.name }}</span
            >
            <span
              v-if="commit.refs.length > MAX_REFS"
              class="commit-ref more"
              :title="commit.refs.map((label) => label.name).join('\n')"
              >+{{ commit.refs.length - MAX_REFS }}</span
            >
            <span class="commit-subject" :title="commit.subject">{{ commit.subject }}</span>
          </span>
          <span
            class="commit-pushed"
            :class="{ local: commit.unpushed }"
            :title="commit.unpushed ? 'Not pushed' : 'Pushed'"
          >
            <GitIcon :name="commit.unpushed ? 'unpushed' : 'pushed'" />
          </span>
          <span class="commit-author" :title="commit.author">{{ commit.author }}</span>
          <span class="commit-hash">{{ commit.hash.slice(0, 7) }}</span>
          <span class="commit-age" :title="formatDate(commit.time)">{{
            formatAge(commit.time)
          }}</span>
        </button>
      </div>

      <div class="history-detail">
        <p v-if="!selectedCommit" class="notice">Select a commit to see what it changed.</p>
        <template v-else>
          <div class="detail-head">
            <p class="detail-message">{{ info?.message ?? selectedCommit.subject }}</p>
            <p class="detail-meta">
              <span class="detail-hash">{{ selectedCommit.hash }}</span>
              <span>{{ selectedCommit.author }}</span>
              <span>{{ formatDate(selectedCommit.time) }}</span>
              <span v-if="selectedCommit.hash === head" class="detail-head-mark">HEAD</span>
            </p>
            <AppSegmented
              v-model="detailMode"
              aria-label="Show"
              :options="[
                { value: 'diff', label: 'Diff' },
                { value: 'blame', label: 'Blame' }
              ]"
            />
          </div>
          <RevisionView
            :project="project"
            :revision="{ kind: 'commit', hash: selectedCommit.hash }"
            :mode="detailMode"
          />
        </template>
      </div>
    </div>

    <AppContextMenu
      v-if="menu"
      :x="menu.x"
      :y="menu.y"
      :items="menuItems"
      @select="onMenuSelect"
      @close="menu = null"
    />
    <HistoryResetModal
      v-if="resetting"
      :project="project"
      :commit="resetting"
      @close="resetting = null"
      @done="afterRewrite"
    />
    <HistoryAmendModal
      v-if="amending"
      :project="project"
      :commit="amending"
      @close="amending = null"
      @done="afterRewrite"
    />
  </div>
</template>

<style scoped>
.history {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.history-bar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-4);
  border-bottom: 1px solid var(--color-border-subtle);
}

.push-error {
  max-height: 120px;
  overflow-y: auto;
  padding: var(--space-2) var(--space-4);
  background: var(--color-error-muted);
  color: var(--color-error);
  font-size: var(--text-xs);
  white-space: pre-wrap;
  word-break: break-word;
}

.history-search {
  flex: 1;
  min-width: 0;
  padding: var(--space-1) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
}

.history-search:focus {
  outline: none;
  border-color: var(--color-amber);
}

.history-body {
  flex: 1;
  min-height: 0;
  display: flex;
}

.history-list {
  flex: 0 0 55%;
  min-width: 0;
  overflow-y: auto;
  border-right: 1px solid var(--color-border-subtle);
  container-type: inline-size;
}

.commit-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  height: 26px;
  padding: 0 var(--space-3) 0 0;
  background: transparent;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.commit-row:hover {
  background: var(--color-surface-2);
}

.commit-row.selected {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.commit-row.head .commit-subject {
  font-weight: var(--weight-semibold);
  color: var(--color-fg-primary);
}

.commit-graph {
  flex-shrink: 0;
  overflow: hidden;
}

.commit-main {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-1);
  overflow: hidden;
}

.commit-subject {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.commit-ref {
  min-width: 0;
  max-width: 120px;
  padding: 0 6px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  border: 1px solid currentColor;
  border-radius: var(--radius-pill);
  line-height: 16px;
}

.commit-ref.head,
.commit-ref.more {
  flex-shrink: 0;
}

.commit-ref.more {
  color: var(--color-fg-muted);
}

.commit-ref.head {
  background: var(--color-amber);
  border-color: var(--color-amber);
  color: var(--color-black);
}

.commit-ref.branch {
  color: var(--color-info);
}

.commit-ref.remote {
  color: var(--color-fg-secondary);
}

.commit-ref.tag {
  color: var(--color-success);
}

.commit-pushed {
  flex-shrink: 0;
  color: var(--color-fg-muted);
}

.commit-pushed {
  display: flex;
}

.commit-pushed.local {
  color: var(--color-warning);
}

.commit-author {
  flex-shrink: 0;
  width: 80px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.commit-hash {
  flex-shrink: 0;
  font-family: var(--font-mono);
  color: var(--color-fg-muted);
}

.commit-age {
  flex-shrink: 0;
  width: 30px;
  color: var(--color-fg-muted);
  text-align: right;
}

/* A narrow pane drops the author and then the hash, so the subject and refs keep the room. */
@container (max-width: 480px) {
  .commit-author {
    display: none;
  }
}

@container (max-width: 360px) {
  .commit-hash {
    display: none;
  }
}

.history-detail {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.detail-head {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4);
  border-bottom: 1px solid var(--color-border-subtle);
}

.detail-message {
  max-height: 120px;
  overflow-y: auto;
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  white-space: pre-wrap;
  word-break: break-word;
}

.detail-meta {
  display: flex;
  flex-wrap: wrap;
  gap: var(--space-3);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.detail-hash {
  font-family: var(--font-mono);
}

.detail-head-mark {
  color: var(--color-fg-accent);
  font-weight: var(--weight-semibold);
}

.notice {
  padding: var(--space-4) var(--space-6);
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
}

.notice.error {
  color: var(--color-error);
  white-space: pre-wrap;
}
</style>
