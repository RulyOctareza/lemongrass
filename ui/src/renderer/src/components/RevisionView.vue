<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import type { ProjectRef } from '../workspace'
import type {
  GitDiff,
  GitFileGroup,
  GitRevFile,
  GitRevFiles,
  GitRevision
} from '../../../preload/types'
import BlameView from './BlameView.vue'
import DiffBody from './DiffBody.vue'

const props = withDefaults(
  defineProps<{
    project: ProjectRef
    revision: GitRevision
    mode?: 'diff' | 'blame'
  }>(),
  { mode: 'diff' }
)

const files = ref<GitRevFiles | null>(null)
const selected = ref<string | null>(null)
const diff = ref<GitDiff | null>(null)
let filesRequest = 0
let diffRequest = 0

const selectedFile = computed(() =>
  files.value?.status === 'ok'
    ? files.value.files.find((f) => f.path === selected.value)
    : undefined
)

function groupOf(file: GitRevFile): GitFileGroup {
  if (file.kind === 'deleted') return 'deleted'
  if (file.kind === 'added') return 'stagedNew'
  if (file.kind === 'renamed' || file.kind === 'copied') return 'renamed'
  return 'changed'
}

function fileName(path: string): string {
  return path.slice(path.lastIndexOf('/') + 1)
}

function fileDir(path: string): string {
  const index = path.lastIndexOf('/')
  return index === -1 ? '' : path.slice(0, index)
}

async function loadFiles(): Promise<void> {
  const id = ++filesRequest
  files.value = null
  selected.value = null
  diff.value = null
  const next = await window.api.git.revFiles(props.project.path, props.revision)
  if (id !== filesRequest) return
  files.value = next
  if (next.status === 'ok' && next.files.length > 0) selected.value = next.files[0].path
}

async function loadDiff(): Promise<void> {
  const file = selectedFile.value
  if (!file) {
    diff.value = null
    return
  }
  const id = ++diffRequest
  diff.value = null
  const next = await window.api.git.revDiff(
    props.project.path,
    props.revision,
    file.path,
    file.origPath
  )
  if (id === diffRequest) diff.value = next
}

watch(() => JSON.stringify(props.revision), loadFiles, { immediate: true })
watch(selected, loadDiff)
</script>

<template>
  <div class="revision">
    <p v-if="!files" class="notice">Loading...</p>
    <p v-else-if="files.status === 'error'" class="notice error">{{ files.message }}</p>
    <p v-else-if="files.files.length === 0" class="notice">This revision changes no files.</p>
    <template v-else>
      <ul class="rev-files">
        <li v-for="file in files.files" :key="file.path">
          <button
            class="rev-file"
            :class="[groupOf(file), { active: file.path === selected }]"
            :title="file.origPath ? `${file.origPath} to ${file.path}` : file.path"
            @click="selected = file.path"
          >
            <span class="rev-name">{{ fileName(file.path) }}</span>
            <span class="rev-dir">{{ fileDir(file.path) }}</span>
          </button>
        </li>
      </ul>
      <div class="rev-diff">
        <BlameView
          v-if="mode === 'blame' && revision.kind === 'commit' && selectedFile"
          :project="project"
          :hash="revision.hash"
          :path="selectedFile.path"
        />
        <DiffBody v-else :diff="diff" />
      </div>
    </template>
  </div>
</template>

<style scoped>
.revision {
  flex: 1;
  min-height: 0;
  display: flex;
}

.rev-files {
  flex-shrink: 0;
  width: 240px;
  overflow-y: auto;
  list-style: none;
  padding: var(--space-2);
  border-right: 1px solid var(--color-border-subtle);
}

.rev-file {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 3px var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.rev-file:hover {
  background: var(--color-surface-2);
}

.rev-file.active {
  background: var(--color-amber-muted);
}

.rev-file.changed,
.rev-file.renamed {
  color: var(--color-info);
}

.rev-file.stagedNew {
  color: var(--color-success);
}

.rev-file.deleted {
  color: var(--color-deleted);
}

.rev-name {
  flex-shrink: 0;
  max-width: 60%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.rev-dir {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  opacity: 0.6;
}

.rev-diff {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
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
