<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import {
  commitChanges,
  fetchAll,
  gitStateOf,
  isChecked,
  pushBranch,
  refreshGit,
  setAllChecked,
  toggleFile,
  toggleGroupFold
} from '../git'
import {
  activeConflictPath,
  activeDiffPath,
  openConflict,
  openDiff,
  openHistory,
  type ProjectRef
} from '../workspace'
import type { GitFile, GitFileGroup, GitFileKind } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppTextarea from './AppTextarea.vue'
import AppCheckbox from './AppCheckbox.vue'
import GitIcon from './GitIcon.vue'
import GitConflictPanel from './GitConflictPanel.vue'
import GitBranchCreateModal from './GitBranchCreateModal.vue'
import GitBranchMenu from './GitBranchMenu.vue'
import GitCounts from './GitCounts.vue'
import GitDivergedModal from './GitDivergedModal.vue'
import GitMergeModal from './GitMergeModal.vue'
import GitStashSection from './GitStashSection.vue'

const POLL_MS = 5000

const props = defineProps<{
  project: ProjectRef
}>()

const rowError = ref<string | null>(null)
const rowNotice = ref<string | null>(null)
const state = computed(() => gitStateOf(props.project.id))
const status = computed(() => state.value.status)
const files = computed(() => status.value?.files ?? [])
const selectedPath = computed(
  () => activeDiffPath(props.project.id) ?? activeConflictPath(props.project.id)
)
const hasConflicts = computed(() => files.value.some((file) => file.kind === 'conflicted'))
// A stopped merge or rebase swaps the body for the conflict panel, since nothing else in the panel
// can be done until it is continued or aborted.
const conflictMode = computed(() => !!status.value?.operation)
// Uncommitted changes, staged ones included, are carried by main, so only states a stash cannot
// carry block the switcher.
const switchBlockedReason = computed((): string | null => {
  if (state.value.busy) return 'An operation is running'
  if (status.value?.operation) {
    return `Continue or abort the ${status.value.operation} before switching branches`
  }
  if (hasConflicts.value) return 'Resolve the conflicts before switching branches'
  return null
})
const checkedCount = computed(
  () => files.value.filter((file) => isChecked(props.project.id, file.path)).length
)
const allChecked = computed(
  () => files.value.length > 0 && checkedCount.value === files.value.length
)
const someChecked = computed(() => checkedCount.value > 0 && !allChecked.value)

const GROUPS: { id: GitFileGroup; label: string }[] = [
  { id: 'conflicted', label: 'Conflicts' },
  { id: 'changed', label: 'Changed' },
  { id: 'renamed', label: 'Renamed' },
  { id: 'stagedNew', label: 'Staged new' },
  { id: 'unstagedNew', label: 'Unstaged new' },
  { id: 'deleted', label: 'Deleted' }
]

const groups = computed(() =>
  GROUPS.map((group) => ({
    ...group,
    files: files.value.filter((file) => file.group === group.id)
  })).filter((group) => group.files.length > 0)
)

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
    !status.value?.operation &&
    checkedCount.value > 0 &&
    state.value.message.trim().length > 0
)

const pushBlockedReason = computed((): string | null => {
  if (status.value?.detached) return 'HEAD is detached'
  if (!status.value?.pushTarget) return 'No remote to push to'
  return null
})

const canPush = computed(() => canCommit.value && pushBlockedReason.value === null)

// Without an upstream the push sets one, so there is nothing to compare ahead against yet.
const pushOnlyBlockedReason = computed((): string | null => {
  if (pushBlockedReason.value) return pushBlockedReason.value
  if (status.value?.upstream && status.value.ahead === 0) return 'Nothing to push'
  return null
})

const pushOnlyTitle = computed(
  () => pushOnlyBlockedReason.value ?? `Push ${status.value?.branch} to ${status.value?.pushTarget}`
)

const fetchTitle = computed(() =>
  status.value?.branch && status.value.upstream
    ? `Fetch every remote and update ${status.value.branch}`
    : 'Fetch every remote'
)

// A conflicted file opens in the resolver, every other file in the diff tab.
function openFile(file: GitFile): void {
  if (file.kind === 'conflicted') openConflict(props.project, file.path)
  else openDiff(props.project, file.path)
}

async function fetchRemotes(): Promise<void> {
  rowError.value = null
  rowNotice.value = null
  const fetched = await fetchAll(props.project)
  rowError.value = fetched.error
  rowNotice.value = fetched.notice
}

async function push(): Promise<void> {
  rowError.value = null
  rowNotice.value = null
  rowError.value = await pushBranch(props.project)
}

const hint = computed((): string | null => {
  if (status.value?.operation) return 'Continue or abort the merge or rebase first.'
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
      :class="{ open: !state.folded, conflict: conflictMode }"
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
      <!-- Unfolded, the branch row below shows these beside Fetch and Push instead. -->
      <GitCounts
        v-if="state.folded && status?.isRepo"
        :ahead="status.ahead"
        :behind="status.behind"
      />
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

    <div v-if="!state.folded && conflictMode" class="git-body">
      <GitConflictPanel :project="project" />
    </div>
    <div v-else-if="!state.folded" class="git-body">
      <GitBranchMenu v-if="status?.isRepo" :project="project" :blocked-reason="switchBlockedReason">
        <GitIcon class="row-icon" name="branch" />
        <span class="row-name">{{ status.branch ?? 'detached HEAD' }}</span>
        <GitCounts :ahead="status.ahead" :behind="status.behind" />
      </GitBranchMenu>
      <div v-if="status?.isRepo" class="remote-line">
        <button class="history-open" @click="openHistory(project)">
          <GitIcon class="row-icon" name="open" />
          <span>History</span>
        </button>
        <span class="remote-spacer" />
        <button
          class="remote-icon fetch"
          :disabled="state.busy !== null"
          :title="fetchTitle"
          :aria-label="fetchTitle"
          @click="fetchRemotes"
        >
          <GitIcon v-if="state.busy === 'fetch'" class="spin" name="spinner" />
          <GitIcon v-else name="fetch" />
        </button>
        <button
          class="remote-icon push"
          :disabled="state.busy !== null || pushOnlyBlockedReason !== null"
          :title="pushOnlyTitle"
          :aria-label="pushOnlyTitle"
          @click="push"
        >
          <GitIcon v-if="state.busy === 'push'" class="spin" name="spinner" />
          <GitIcon v-else name="push" />
        </button>
      </div>
      <p v-if="rowError" class="git-error">{{ rowError }}</p>
      <p v-if="rowNotice" class="git-notice">{{ rowNotice }}</p>
      <div class="changes-head">
        <AppCheckbox
          v-if="files.length > 0"
          class="file-check"
          title="Select all"
          aria-label="Select all changes"
          :checked="allChecked"
          :indeterminate="someChecked"
          @click="setAllChecked(project.id, files, !allChecked)"
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
        <div v-else class="file-list">
          <section v-for="group in groups" :key="group.id" class="file-group">
            <button
              class="group-head"
              :aria-expanded="!state.foldedGroups.includes(group.id)"
              @click="toggleGroupFold(project.id, group.id)"
            >
              <svg
                class="group-chevron"
                :class="{ open: !state.foldedGroups.includes(group.id) }"
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
              <span>{{ group.label }}</span>
              <span class="group-count">{{ group.files.length }}</span>
            </button>
            <ul v-if="!state.foldedGroups.includes(group.id)" class="group-files">
              <li v-for="file in group.files" :key="file.path" class="file-item">
                <AppCheckbox
                  class="file-check"
                  :aria-label="`Include ${file.path} in the commit`"
                  :checked="isChecked(project.id, file.path)"
                  @click="toggleFile(project.id, file.path)"
                />
                <button
                  class="file-row"
                  :class="[file.group, { active: file.path === selectedPath }]"
                  :title="fileTitle(file)"
                  @click="openFile(file)"
                >
                  <span class="file-name">{{ fileName(file) }}</span>
                  <span class="file-dir">{{ fileDir(file) }}</span>
                </button>
              </li>
            </ul>
          </section>
        </div>

        <GitStashSection :project="project" :has-changes="files.length > 0" />

        <AppTextarea
          v-model="state.message"
          size="sm"
          rows="3"
          placeholder="Commit message"
          :disabled="state.busy !== null"
        />
        <p v-if="state.error" class="git-error">{{ state.error }}</p>
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

    <GitBranchCreateModal
      v-if="state.branchBase"
      :project="project"
      :initial-base="state.branchBase"
      :has-changes="files.length > 0"
      @close="state.branchBase = null"
    />
    <GitMergeModal
      v-if="state.mergeSource"
      :project="project"
      :source="state.mergeSource"
      @close="state.mergeSource = null"
    />
    <GitDivergedModal
      v-if="state.diverged"
      :project="project"
      :diverged="state.diverged"
      @close="state.diverged = null"
    />
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
  background: var(--layer-raised);
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
  background: var(--layer-raised-hover);
}

.git-item.open {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.git-item.conflict,
.git-item.conflict:hover {
  background: var(--color-error-muted);
  color: var(--color-error);
}

.git-item.conflict .git-count {
  background: var(--color-error);
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

.remote-line {
  display: flex;
  align-items: center;
  gap: var(--space-1);
  min-width: 0;
}

.remote-spacer {
  flex: 1;
}

.remote-icon {
  --tint: var(--color-fg-muted);
  display: flex;
  align-items: center;
  justify-content: center;
  width: 20px;
  height: 20px;
  flex-shrink: 0;
  padding: 0;
  background: color-mix(in srgb, var(--tint) 14%, transparent);
  border: none;
  border-radius: var(--radius-sm);
  color: var(--tint);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

/* Fetch matches the blue behind count and Push the green ahead count. */
.remote-icon.fetch {
  --tint: var(--color-info);
}

.remote-icon.push {
  --tint: var(--color-success);
}

.remote-icon:hover:not(:disabled),
.remote-icon:focus-visible:not(:disabled) {
  background: color-mix(in srgb, var(--tint) 28%, transparent);
}

.remote-icon:disabled {
  opacity: 0.5;
  cursor: default;
}

.remote-icon:has(.spin) {
  opacity: 1;
}

.history-open {
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
  cursor: pointer;
}

.history-open:hover {
  background: var(--color-surface-2);
}

.row-icon {
  color: var(--color-fg-muted);
}

.row-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.history-open:hover .row-icon,
.history-open:focus-visible .row-icon {
  color: var(--color-fg-accent);
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

/* The panel rows are compact, so the checkbox drops its own vertical padding. */
.file-check {
  flex-shrink: 0;
  padding: 0;
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
}

.group-head {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 3px var(--space-2);
  background: transparent;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
}

.group-head:hover {
  color: var(--color-fg-primary);
}

.group-chevron {
  flex-shrink: 0;
  transition: transform var(--duration-fast) var(--ease-out);
}

.group-chevron.open {
  transform: rotate(90deg);
}

.group-count {
  color: var(--color-fg-muted);
  font-weight: var(--weight-regular);
}

.group-files {
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

/* Name and directory take the group color, so the group is readable without a letter. */
.file-row.changed,
.file-row.renamed {
  color: var(--color-info);
}

.file-row.stagedNew {
  color: var(--color-success);
}

.file-row.unstagedNew {
  color: var(--color-error);
}

.file-row.deleted {
  color: var(--color-deleted);
}

.file-row.conflicted {
  color: var(--color-warning);
}

.file-row.changed .file-dir,
.file-row.renamed .file-dir,
.file-row.stagedNew .file-dir,
.file-row.unstagedNew .file-dir,
.file-row.deleted .file-dir,
.file-row.conflicted .file-dir {
  color: inherit;
  opacity: 0.6;
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

.git-notice {
  padding: var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
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
