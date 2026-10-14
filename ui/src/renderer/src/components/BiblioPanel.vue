<script setup lang="ts">
import { computed, ref } from 'vue'
import AppModal from './AppModal.vue'
import BiblioTreeItem from './BiblioTreeItem.vue'
import GitPanel from './GitPanel.vue'
import MarkdownEditor from './MarkdownEditor.vue'
import NodeEntry from './NodeEntry.vue'
import VaultEntry from './VaultEntry.vue'
import WorkgroupPanel from './WorkgroupPanel.vue'
import { activePath, closeUnder, openFile, type ProjectRef } from '../workspace'
import type { BiblioTree } from '../../../preload/types'

const props = defineProps<{
  tree: BiblioTree | null
  project: ProjectRef
}>()

const emit = defineEmits<{
  refresh: []
  openVault: []
}>()

const selectedPath = computed((): string | null => activePath(props.project.id))

function selectFile(path: string): void {
  openFile(props.project, path)
}

async function archiveScratchpad(taskPath: string): Promise<void> {
  const ok = await window.api.biblio.archiveScratchpad(props.project.path, taskPath)
  if (!ok) return
  closeUnder(props.project, taskPath)
  emit('refresh')
}

const showCreateForm = ref(false)
const newTitle = ref('')
const newContent = ref('')
const creating = ref(false)
const pendingImages = new Map<string, File>()

function clearPendingImages(): void {
  for (const url of pendingImages.keys()) URL.revokeObjectURL(url)
  pendingImages.clear()
}

async function storePendingImages(notePath: string, content: string): Promise<void> {
  let resolved = content
  for (const [url, file] of pendingImages) {
    if (!resolved.includes(url)) continue
    const bytes = new Uint8Array(await file.arrayBuffer())
    const result = await window.api.biblio.saveScratchpadImage(
      props.project.path,
      notePath,
      bytes,
      file.type
    )
    if ('path' in result) resolved = resolved.replaceAll(url, result.path)
  }
  if (resolved !== content) await window.api.biblio.write(props.project.path, notePath, resolved)
}

// Non-null when the create form targets an existing folder instead of a new task directory.
const createTargetFolder = ref<string | null>(null)

function openCreateForm(): void {
  clearPendingImages()
  newTitle.value = ''
  newContent.value = ''
  createTargetFolder.value = null
  showCreateForm.value = true
}

function openCreateFileForm(folderPath: string): void {
  clearPendingImages()
  newTitle.value = ''
  newContent.value = ''
  createTargetFolder.value = folderPath
  showCreateForm.value = true
}

function cancelCreate(): void {
  clearPendingImages()
  showCreateForm.value = false
}

async function submitCreate(): Promise<void> {
  if (!newTitle.value.trim() || creating.value) return
  creating.value = true
  const path = createTargetFolder.value
    ? await window.api.biblio.createScratchpadFile(
        props.project.path,
        createTargetFolder.value,
        newTitle.value,
        newContent.value
      )
    : await window.api.biblio.createScratchpad(props.project.path, newTitle.value, newContent.value)
  if (path) await storePendingImages(path, newContent.value)
  creating.value = false
  if (!path) return
  clearPendingImages()
  showCreateForm.value = false
  emit('refresh')
  selectFile(path)
}

const treeWidth = ref(240)
let dragStartX = 0
let dragStartWidth = 0

function onGutterDown(event: PointerEvent): void {
  dragStartX = event.clientX
  dragStartWidth = treeWidth.value
  window.addEventListener('pointermove', onGutterMove)
  window.addEventListener('pointerup', onGutterUp)
}

function onGutterMove(event: PointerEvent): void {
  const delta = event.clientX - dragStartX
  treeWidth.value = Math.min(480, Math.max(180, dragStartWidth + delta))
}

function onGutterUp(): void {
  window.removeEventListener('pointermove', onGutterMove)
  window.removeEventListener('pointerup', onGutterUp)
}
</script>

<template>
  <div class="biblio-pane">
    <div class="biblio-tree" :style="{ width: `${treeWidth}px` }">
      <div class="tree-scroll">
        <button
          v-if="tree?.toc"
          class="toc-item"
          :class="{ active: tree.toc.path === selectedPath }"
          @click="selectFile(tree.toc.path)"
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
            <line x1="2" y1="3" x2="12" y2="3" />
            <line x1="2" y1="7" x2="12" y2="7" />
            <line x1="2" y1="11" x2="8" y2="11" />
          </svg>
          <span>Table of Content</span>
        </button>

        <div class="tree-groups">
          <template v-if="tree?.children?.length">
            <BiblioTreeItem
              v-for="child in tree.children"
              :key="child.path"
              :node="child"
              :selected-path="selectedPath"
              :depth="0"
              @select="selectFile"
              @add-scratchpad="openCreateForm"
              @add-scratchpad-file="openCreateFileForm"
              @archive-scratchpad="archiveScratchpad"
            />
          </template>
          <p v-else class="empty">Nothing in biblio/ yet.</p>
        </div>
      </div>

      <WorkgroupPanel :project="project" />
      <GitPanel :project="project" />
      <VaultEntry @open="emit('openVault')" />
      <NodeEntry />
    </div>

    <div class="biblio-gutter" @pointerdown="onGutterDown" />

    <AppModal
      v-if="showCreateForm"
      scope="app"
      size="md"
      :title="createTargetFolder ? 'New file' : 'New scratchpad'"
      @close="cancelCreate"
    >
      <input
        v-model="newTitle"
        class="title-input"
        type="text"
        placeholder="Title"
        autofocus
        @keydown.enter="submitCreate"
      />
      <MarkdownEditor
        v-model="newContent"
        @image-pending="(url, file) => pendingImages.set(url, file)"
      />
      <template #footer>
        <button class="ghost-button" @click="cancelCreate">Cancel</button>
        <button
          class="primary-button"
          :disabled="!newTitle.trim() || creating"
          @click="submitCreate"
        >
          {{ creating ? 'Creating...' : 'Create' }}
        </button>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.biblio-pane {
  flex-shrink: 0;
  min-height: 0;
  display: flex;
}

.title-input {
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
}

.title-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.ghost-button,
.primary-button {
  padding: var(--space-1) var(--space-4);
  border-radius: var(--radius-pill);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.ghost-button {
  background: transparent;
  border: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
}

.ghost-button:hover {
  color: var(--color-fg-primary);
  background: var(--color-surface-2);
}

.primary-button {
  background: var(--color-amber);
  border: none;
  color: var(--color-black);
}

.primary-button:hover:not(:disabled) {
  background: var(--color-amber-dim);
}

.primary-button:disabled {
  opacity: 0.5;
  cursor: default;
}

.biblio-tree {
  flex-shrink: 0;
  min-height: 0;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-1);
}

.tree-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-3) var(--space-2) var(--space-4);
}

.toc-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  margin-bottom: var(--space-3);
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

.toc-item:hover {
  background: var(--color-surface-3);
}

.toc-item.active {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.tree-groups {
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.biblio-gutter {
  flex-shrink: 0;
  width: 4px;
  cursor: col-resize;
  background: var(--color-border-subtle);
  transition: background var(--duration-fast) var(--ease-out);
}

.biblio-gutter:hover,
.biblio-gutter:active {
  background: var(--color-amber);
}

.empty {
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
  padding: var(--space-4);
}
</style>
