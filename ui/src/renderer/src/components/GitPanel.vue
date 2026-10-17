<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { commitChanges, gitStateOf, isChecked, refreshGit, setAllChecked, toggleFile } from '../git'
import { activeDiffPath, openDiff, type ProjectRef } from '../workspace'
import type { GitFile, GitFileKind } from '../../../preload/types'
import AppButton from './AppButton.vue'

const POLL_MS = 5000

const props = defineProps<{
  project: ProjectRef
}>()

const state = computed(() => gitStateOf(props.project.id))
const status = computed(() => state.value.status)
const files = computed(() => status.value?.files ?? [])
const selectedPath = computed(() => activeDiffPath(props.project.id))
const hasConflicts = computed(() => files.value.some((file) => file.kind === 'conflicted'))
const checkedCount = computed(
  () => files.value.filter((file) => isChecked(props.project.id, file.path)).length
)
const allChecked = computed(
  () => files.value.length > 0 && checkedCount.value === files.value.length
)
const someChecked = computed(() => checkedCount.value > 0 && !allChecked.value)

const KIND_LETTERS: Record<GitFileKind, string> = {
  modified: 'M',
  added: 'A',
  deleted: 'D',
  renamed: 'R',
  copied: 'C',
  typechange: 'T',
  untracked: 'U',
  conflicted: '!'
}

const KIND_LABELS: Record<GitFileKind, string> = {
  modified: 'Modified',
  added: 'Added',
  deleted: 'Deleted',
  renamed: 'Renamed',
  copied: 'Copied',
  typechange: 'Type changed',
  untracked: 'Untracked',
  conflicted: 'Conflicted'
}

function fileName(file: GitFile): string {
  return file.path.slice(file.path.lastIndexOf('/') + 1)
}

function fileDir(file: GitFile): string {
  const index = file.path.lastIndexOf('/')
  return index === -1 ? '' : file.path.slice(0, index)
}

function fileTitle(file: GitFile): string {
  const label = KIND_LABELS[file.kind]
  return file.origPath ? `${label}: ${file.origPath} to ${file.path}` : `${label}: ${file.path}`
}

const canCommit = computed(
  () =>
    !state.value.busy &&
    !hasConflicts.value &&
    checkedCount.value > 0 &&
    state.value.message.trim().length > 0
)

const pushBlockedReason = computed((): string | null => {
  if (status.value?.detached) return 'HEAD is detached'
  if (!status.value?.pushTarget) return 'No remote to push to'
  return null
})

const canPush = computed(() => canCommit.value && pushBlockedReason.value === null)

const hint = computed((): string | null => {
  if (hasConflicts.value) return 'Resolve conflicts before committing.'
  if (state.value.busy === 'commit') return 'Committing...'
  if (state.value.busy === 'push') return 'Pushing...'
  return null
})

const pushTitle = computed(() => {
  if (pushBlockedReason.value) return pushBlockedReason.value
  return `Commit, then push to ${status.value?.pushTarget}`
})

function refresh(): void {
  void refreshGit(props.project)
}

function onFocus(): void {
  refresh()
}

let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  refresh()
  timer = setInterval(() => {
    if (document.hasFocus()) refresh()
  }, POLL_MS)
  window.addEventListener('focus', onFocus)
})

onBeforeUnmount(() => {
  clearInterval(timer)
  window.removeEventListener('focus', onFocus)
})

watch(() => props.project.id, refresh)
</script>

<template>
  <div class="git-panel">
    <button
      class="git-item"
      :class="{ open: !state.folded }"
      :aria-expanded="!state.folded"
      @click="state.folded = !state.folded"
    >
      <svg
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="4" cy="3" r="1.5" />
        <circle cx="4" cy="11" r="1.5" />
        <circle cx="10.5" cy="5.5" r="1.5" />
        <path d="M4 4.5v5M10.5 7c0 2.2-2.2 2.9-6 3.2" />
      </svg>
      <span class="git-label">Git</span>
      <span v-if="status?.branch" class="git-branch" :title="status.branch">{{
        status.branch
      }}</span>
      <span v-if="files.length" class="git-count">{{ files.length }}</span>
      <svg
        class="git-chevron"
        width="10"
        height="10"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M3 2l3 3-3 3" />
      </svg>
    </button>

    <div v-if="!state.folded" class="git-body">
      <div class="changes-head">
        <input
          v-if="files.length > 0"
          type="checkbox"
          class="file-check"
          title="Select all"
          aria-label="Select all changes"
          :checked="allChecked"
          :indeterminate.prop="someChecked"
          @change="setAllChecked(project.id, files, !allChecked)"
        />
        <p class="git-heading">Changes</p>
        <span v-if="files.length > 0" class="git-selected"
          >{{ checkedCount }} of {{ files.length }}</span
        >
      </div>
      <p v-if="!status" class="git-empty">Loading...</p>
      <p v-else-if="!status.isRepo" class="git-empty">Not a git repository.</p>
      <template v-else>
        <p v-if="files.length === 0" class="git-empty">No changes</p>
        <ul v-else class="file-list lg-scroll">
          <li v-for="file in files" :key="file.path" class="file-item">
            <input
              type="checkbox"
              class="file-check"
              :aria-label="`Include ${file.path} in the commit`"
              :checked="isChecked(project.id, file.path)"
              @change="toggleFile(project.id, file.path)"
            />
            <button
              class="file-row"
              :class="{ active: file.path === selectedPath }"
              :title="fileTitle(file)"
              @click="openDiff(project, file.path)"
            >
              <span class="file-kind" :class="file.kind">{{ KIND_LETTERS[file.kind] }}</span>
              <span class="file-name">{{ fileName(file) }}</span>
              <span class="file-dir">{{ fileDir(file) }}</span>
            </button>
          </li>
        </ul>

        <textarea
          v-model="state.message"
          class="message-input"
          rows="3"
          placeholder="Commit message"
          :disabled="state.busy !== null"
        />
        <p v-if="state.error" class="git-error lg-scroll">{{ state.error }}</p>
        <p v-if="hint" class="git-hint">{{ hint }}</p>
        <div class="git-actions">
          <AppButton
            class="git-action"
            :disabled="!canCommit"
            @click="commitChanges(project, false)"
          >
            Commit
          </AppButton>
          <AppButton
            variant="primary"
            class="git-action"
            :disabled="!canPush"
            :title="pushTitle"
            @click="commitChanges(project, true)"
          >
            Commit and push
          </AppButton>
        </div>
      </template>
    </div>
  </div>
</template>

<style scoped>
.git-panel {
  flex-shrink: 0;
  padding: var(--space-2);
  border-top: 1px solid var(--color-border-default);
}

.git-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.git-item:hover {
  background: var(--color-surface-3);
}

.git-item.open {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.git-label {
  flex-shrink: 0;
}

.git-branch {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-secondary);
  font-weight: var(--weight-regular);
  text-align: left;
}

.git-count {
  flex-shrink: 0;
  min-width: 18px;
  padding: 0 6px;
  border-radius: var(--radius-pill);
  background: var(--color-amber);
  color: var(--color-black);
  font-size: 11px;
  line-height: 18px;
  text-align: center;
}

.git-chevron {
  flex-shrink: 0;
  margin-left: auto;
  transition: transform var(--duration-fast) var(--ease-out);
}

.git-item.open .git-chevron {
  transform: rotate(90deg);
}

.git-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  max-height: 55vh;
  overflow-y: auto;
  padding: var(--space-2) var(--space-1) 0;
}

.changes-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 var(--space-2);
}

.git-heading {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.git-selected {
  margin-left: auto;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.file-check {
  flex-shrink: 0;
  width: 13px;
  height: 13px;
  margin: 0;
  accent-color: var(--color-amber);
  cursor: pointer;
}

.file-item {
  display: flex;
  align-items: center;
  gap: 2px;
  padding-left: var(--space-2);
}

.git-empty {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.file-list {
  flex-shrink: 0;
  max-height: 180px;
  overflow-y: auto;
  list-style: none;
  margin: 0;
  padding: 0;
}

.file-row {
  display: flex;
  align-items: center;
  gap: 6px;
  flex: 1;
  min-width: 0;
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

.file-row:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.file-row.active {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.file-kind {
  flex-shrink: 0;
  width: 12px;
  font-family: var(--font-mono);
  font-weight: var(--weight-bold);
  text-align: center;
}

.file-kind.modified,
.file-kind.typechange {
  color: var(--color-warning);
}

.file-kind.added,
.file-kind.untracked {
  color: var(--color-success);
}

.file-kind.deleted,
.file-kind.conflicted {
  color: var(--color-error);
}

.file-kind.renamed,
.file-kind.copied {
  color: var(--color-info);
}

.file-name {
  flex-shrink: 0;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.file-dir {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
  text-align: left;
}

.message-input {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  resize: vertical;
}

.message-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.git-error {
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

.git-hint {
  padding: 0 var(--space-2);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.git-actions {
  display: flex;
  gap: var(--space-2);
  padding-bottom: var(--space-1);
}

/* The two actions split the narrow sidebar evenly, so they take less side padding than elsewhere. */
.git-actions .git-action {
  flex: 1;
  padding: var(--space-1) var(--space-2);
}
</style>
