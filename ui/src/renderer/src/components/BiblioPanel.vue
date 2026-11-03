<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import AppModal from './AppModal.vue'
import BiblioTreeItem from './BiblioTreeItem.vue'
import MarkdownEditor from './MarkdownEditor.vue'
import SearchRows from './treeSearch/SearchRows.vue'
import TreeSearchBar from './treeSearch/TreeSearchBar.vue'
import { activePath, closeUnder, openFile, type ProjectRef } from '../workspace'
import { biblioExpandedOf } from '../explorerStore'
import { biblioIconOf } from '../biblioIcons'
import { useTreeSearch } from '../useTreeSearch'
import type { SearchCandidate, SearchNode, SearchRow } from '../treeSearch'
import type { BiblioNode, BiblioTree } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppInput from './AppInput.vue'
import AppEmptyState from './AppEmptyState.vue'

const props = defineProps<{
  tree: BiblioTree | null
  project: ProjectRef
}>()

const emit = defineEmits<{
  refresh: []
}>()

const selectedPath = computed((): string | null => activePath(props.project.id))

const expandedPaths = computed(() => biblioExpandedOf(props.project.id))

function selectFile(path: string): void {
  openFile(props.project, path)
}

function toggleDir(path: string): void {
  const expanded = biblioExpandedOf(props.project.id)
  if (expanded[path]) delete expanded[path]
  else expanded[path] = true
}

function expandDir(path: string): void {
  biblioExpandedOf(props.project.id)[path] = true
}

const panel = ref<HTMLElement | null>(null)

function collect(nodes: BiblioNode[], out: SearchCandidate[]): SearchCandidate[] {
  for (const node of nodes) {
    out.push({ path: node.path, type: node.type })
    if (node.children) collect(node.children, out)
  }
  return out
}

// A folder result opens the folders down to it in the tree and leaves the search; a file opens
// in the focused pane and the search stays, so the next match is one arrow away.
function openResult(row: SearchRow): void {
  if (row.node.type === 'file') {
    selectFile(row.node.path)
    return
  }
  const segments = row.node.path.split('/')
  for (let i = 1; i <= segments.length; i++) expandDir(segments.slice(0, i).join('/'))
  search.close()
}

const search = useTreeSearch({
  panel,
  candidates: () => ({
    items: collect(props.tree?.toc ? [props.tree.toc] : [], collect(props.tree?.children ?? [], []))
  }),
  open: openResult
})

// The tree refetches when biblio/ changes on disk, and an open search follows it.
watch(
  () => props.tree,
  () => search.refresh()
)

function resultLabel(node: SearchNode): string {
  return node.type === 'file' ? node.name.replace(/\.md$/, '') : node.name
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
</script>

<template>
  <div ref="panel" class="biblio-view" tabindex="0" @keydown="search.onPanelKeydown">
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

      <TreeSearchBar v-if="search.open.value" :search="search" placeholder="Search biblio" />

      <SearchRows
        v-if="search.active.value"
        :search="search"
        :icon-of="(node) => biblioIconOf(node.path, node.type)"
        :label-of="resultLabel"
        :active-path="selectedPath"
      />
      <div v-else class="tree-groups">
        <template v-if="tree?.children?.length">
          <BiblioTreeItem
            v-for="child in tree.children"
            :key="child.path"
            :node="child"
            :selected-path="selectedPath"
            :depth="0"
            :expanded-paths="expandedPaths"
            @select="selectFile"
            @toggle="toggleDir"
            @expand="expandDir"
            @add-scratchpad="openCreateForm"
            @add-scratchpad-file="openCreateFileForm"
            @archive-scratchpad="archiveScratchpad"
          />
        </template>
        <AppEmptyState v-else class="tree-empty" size="sm" title="Nothing in biblio/ yet." />
      </div>
    </div>

    <AppModal
      v-if="showCreateForm"
      scope="app"
      size="md"
      :title="createTargetFolder ? 'New file' : 'New scratchpad'"
      @close="cancelCreate"
    >
      <AppInput
        v-model="newTitle"
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
        <AppButton @click="cancelCreate">Cancel</AppButton>
        <AppButton variant="primary" :disabled="!newTitle.trim() || creating" @click="submitCreate">
          {{ creating ? 'Creating...' : 'Create' }}
        </AppButton>
      </template>
    </AppModal>
  </div>
</template>

<style scoped>
.biblio-view {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  outline: none;
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

.tree-empty {
  margin: var(--space-2);
}
</style>
