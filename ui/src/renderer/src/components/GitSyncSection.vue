<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { gitStateOf, runSync, syncTargetOf } from '../git'
import { formatAge } from '../gitTime'
import type { ProjectRef } from '../workspace'
import type { GitIncomingCommit } from '../../../preload/types'
import AppButton from './AppButton.vue'
import GitBranchModal from './GitBranchModal.vue'
import GitIcon from './GitIcon.vue'

const props = defineProps<{
  project: ProjectRef
}>()

const state = computed(() => gitStateOf(props.project.id))
const status = computed(() => state.value.status)
const target = computed(() => syncTargetOf(props.project.id))
const folded = ref(true)
const picking = ref(false)
const error = ref<string | null>(null)
const confirmingRebase = ref(false)
const incoming = ref<GitIncomingCommit[]>([])
const operationLabel = computed(() => (status.value?.operation === 'merge' ? 'Merge' : 'Rebase'))
let requestId = 0

async function loadIncoming(): Promise<void> {
  const id = ++requestId
  const branch = target.value
  const next = branch ? await window.api.git.incoming(props.project.path, branch) : []
  if (id !== requestId) return
  if (JSON.stringify(next) !== JSON.stringify(incoming.value)) incoming.value = next
}

watch(
  () => [folded.value, state.value.revision, target.value],
  () => {
    confirmingRebase.value = false
    if (!folded.value) void loadIncoming()
  },
  { immediate: true }
)

async function perform(action: 'fetch' | 'merge' | 'rebase' | 'continue' | 'abort'): Promise<void> {
  error.value = null
  error.value = await runSync(props.project, action)
}

async function rebase(): Promise<void> {
  if (!confirmingRebase.value) {
    confirmingRebase.value = true
    return
  }
  confirmingRebase.value = false
  await perform('rebase')
}

const canIntegrate = computed(
  () => !state.value.busy && !status.value?.operation && incoming.value.length > 0
)
</script>

<template>
  <div class="sync-section">
    <div class="sync-head">
      <button class="sync-toggle" :aria-expanded="!folded" @click="folded = !folded">
        <svg
          class="sync-chevron"
          :class="{ open: !folded }"
          width="9"
          height="9"
          viewBox="0 0 10 10"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M3 2l3 3-3 3" />
        </svg>
        <span class="sync-heading">Sync</span>
        <span v-if="status && (status.behind > 0 || status.ahead > 0)" class="sync-counts">
          {{ status.behind }} behind, {{ status.ahead }} ahead
        </span>
      </button>
      <button
        class="sync-fetch"
        :disabled="state.busy !== null"
        title="Fetch every remote"
        aria-label="Fetch every remote"
        @click="perform('fetch')"
      >
        <GitIcon :class="{ spin: state.busy === 'sync' }" name="fetch" />
      </button>
    </div>

    <div v-if="status?.operation" class="sync-operation">
      <p>{{ operationLabel }} in progress. Resolve the conflicts, then continue or abort.</p>
      <div class="sync-buttons">
        <AppButton
          variant="primary"
          class="sync-action"
          :disabled="
            state.busy !== null || status.files.some((file) => file.group === 'conflicted')
          "
          @click="perform('continue')"
        >
          Continue
        </AppButton>
        <AppButton
          variant="danger"
          class="sync-action"
          :disabled="state.busy !== null"
          @click="perform('abort')"
        >
          Abort
        </AppButton>
      </div>
    </div>

    <template v-if="!folded">
      <button
        class="sync-target"
        :disabled="state.busy !== null"
        title="Change the reference branch"
        aria-label="Change the reference branch"
        @click="picking = true"
      >
        <span class="sync-target-label">Reference</span>
        <span class="sync-target-name">{{ target ?? 'no upstream, choose a branch' }}</span>
        <GitIcon class="sync-target-action" name="change" />
      </button>
      <p v-if="error" class="sync-error">{{ error }}</p>
      <p v-if="!target" class="sync-empty">The current branch has no upstream.</p>
      <p v-else-if="incoming.length === 0" class="sync-empty">
        Nothing to bring in from {{ target }}.
      </p>
      <template v-else>
        <p class="sync-incoming">{{ incoming.length }} incoming from {{ target }}</p>
        <ul class="sync-list">
          <li v-for="commit in incoming" :key="commit.hash" class="sync-commit">
            <span class="sync-subject" :title="`${commit.author}: ${commit.subject}`">{{
              commit.subject
            }}</span>
            <span class="sync-age">{{ commit.author }}, {{ formatAge(commit.time) }}</span>
          </li>
        </ul>
      </template>
      <div class="sync-buttons">
        <AppButton
          class="sync-action"
          :disabled="!canIntegrate"
          :title="`Merge ${target} into ${status?.branch}`"
          @click="perform('merge')"
        >
          Merge
        </AppButton>
        <AppButton
          variant="danger"
          class="sync-action"
          :disabled="!canIntegrate"
          :title="`Replay ${status?.branch} on top of ${target}`"
          @click="rebase"
        >
          {{ confirmingRebase ? 'Confirm rebase' : 'Rebase' }}
        </AppButton>
      </div>
      <p v-if="confirmingRebase" class="sync-warning">
        Rebase rewrites the local commits of {{ status?.branch }}. A commit that is already pushed
        cannot be pushed again without a force push, which this app never does.
      </p>
    </template>

    <GitBranchModal
      v-if="picking"
      :project="project"
      pick-only
      title="Choose the reference branch"
      @pick="state.syncTarget = $event"
      @close="picking = false"
    />
  </div>
</template>

<style scoped>
.sync-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.sync-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 var(--space-2);
}

.sync-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 0;
  background: none;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  cursor: pointer;
}

.sync-heading {
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.sync-chevron {
  transition: transform var(--duration-fast) var(--ease-out);
}

.sync-chevron.open {
  transform: rotate(90deg);
}

.sync-counts {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.sync-fetch {
  display: flex;
  margin-left: auto;
  padding: 0;
  background: none;
  border: none;
  color: var(--color-fg-muted);
  cursor: pointer;
}

.sync-fetch:hover:not(:disabled),
.sync-fetch:focus-visible:not(:disabled) {
  color: var(--color-fg-accent);
}

.sync-fetch:disabled {
  cursor: default;
}

.sync-operation {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0 var(--space-2);
  padding: var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  color: var(--color-warning);
  font-size: var(--text-xs);
}

.sync-target {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 3px var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.sync-target:hover:not(:disabled) {
  background: var(--color-surface-2);
}

.sync-target-label {
  flex-shrink: 0;
  font-weight: var(--weight-semibold);
}

.sync-target-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-primary);
}

.sync-target-action {
  color: var(--color-fg-muted);
}

.sync-target:hover:not(:disabled) .sync-target-action,
.sync-target:focus-visible:not(:disabled) .sync-target-action {
  color: var(--color-fg-accent);
}

.sync-incoming {
  padding: 0 var(--space-2);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.sync-empty {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.sync-list {
  max-height: 140px;
  overflow-y: auto;
  list-style: none;
}

.sync-commit {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 2px var(--space-2);
  font-size: var(--text-xs);
}

.sync-subject {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-secondary);
}

.sync-age {
  flex-shrink: 0;
  max-width: 45%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
}

.sync-buttons {
  display: flex;
  gap: var(--space-2);
  padding: 0 var(--space-2);
}

.sync-operation .sync-buttons {
  padding: 0;
}

.sync-buttons .sync-action {
  flex: 1;
  padding: var(--space-1) var(--space-2);
}

.sync-warning {
  padding: 0 var(--space-2);
  color: var(--color-warning);
  font-size: var(--text-xs);
}

.sync-error {
  max-height: 120px;
  overflow-y: auto;
  padding: var(--space-2);
  background: var(--color-error-muted);
  border-radius: var(--radius-md);
  color: var(--color-error);
  font-size: var(--text-xs);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
