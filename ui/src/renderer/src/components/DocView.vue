<script setup lang="ts">
import { computed } from 'vue'
import MarkdownEditor from './MarkdownEditor.vue'
import TocViewer from './TocViewer.vue'
import { editDoc, workspaceOf, type ProjectRef } from '../workspace'
import type { WorkspaceTab } from '../../../preload/types'

type DocTab = Extract<WorkspaceTab, { kind: 'doc' }>

const props = defineProps<{
  project: ProjectRef
  tab: DocTab
}>()

const doc = computed(() => workspaceOf(props.project.id)?.docs[props.tab.path])

function edit(value: string): void {
  editDoc(props.project, props.tab.path, value)
}
</script>

<template>
  <div class="doc-view">
    <p v-if="!doc || doc.status === 'loading'" class="empty">Loading...</p>
    <p v-else-if="doc.status === 'missing'" class="empty">
      This file no longer exists. Close the tab to remove it.
    </p>
    <div v-else-if="doc.editable" class="editor-wrap">
      <MarkdownEditor
        :key="tab.id"
        :model-value="doc.content"
        :project-path="project.path"
        :note-path="tab.path"
        @update:model-value="edit"
      />
    </div>
    <TocViewer v-else-if="tab.path === 'books/toc.md'" :project="project" />
    <div v-else class="reader">
      <!-- eslint-disable-next-line vue/no-v-html -->
      <div class="markdown-body" v-html="doc.html" />
    </div>
  </div>
</template>

<style scoped>
.doc-view {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.empty {
  color: var(--color-fg-muted);
  font-size: var(--text-sm);
  padding: var(--space-4) var(--space-6);
}

.editor-wrap {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  padding: var(--space-3) var(--space-4) var(--space-4);
}

.editor-wrap :deep(.markdown-editor) {
  flex: 1;
  max-width: none;
}

.reader {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding: var(--space-4) var(--space-6);
}

.markdown-body {
  width: 100%;
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.markdown-body :deep(h1),
.markdown-body :deep(h2),
.markdown-body :deep(h3) {
  font-family: var(--font-display);
  font-weight: var(--weight-bold);
  line-height: var(--leading-tight);
  margin-top: var(--space-6);
  margin-bottom: var(--space-3);
}

.markdown-body :deep(h1:first-child),
.markdown-body :deep(h2:first-child),
.markdown-body :deep(h3:first-child) {
  margin-top: 0;
}

.markdown-body :deep(h1) {
  font-size: var(--text-xl);
}
.markdown-body :deep(h2) {
  font-size: var(--text-lg);
}
.markdown-body :deep(h3) {
  font-size: var(--text-md);
}

.markdown-body :deep(p) {
  margin-bottom: var(--space-4);
}

.markdown-body :deep(ul),
.markdown-body :deep(ol) {
  margin-bottom: var(--space-4);
  padding-left: var(--space-6);
}

.markdown-body :deep(li) {
  margin-bottom: var(--space-1);
}

.markdown-body :deep(a) {
  color: var(--color-fg-accent);
}

.markdown-body :deep(strong) {
  font-weight: var(--weight-semibold);
}

.markdown-body :deep(code) {
  font-family: var(--font-mono);
  font-size: 0.9em;
  background: var(--color-surface-2);
  padding: 0.15em 0.4em;
  border-radius: var(--radius-sm);
}

.markdown-body :deep(pre) {
  background: var(--color-surface-2);
  border-radius: var(--radius-lg);
  padding: var(--space-4);
  overflow-x: auto;
  margin-bottom: var(--space-4);
}

.markdown-body :deep(pre code) {
  background: none;
  padding: 0;
}

.markdown-body :deep(blockquote) {
  border-left: 2px solid var(--color-border-default);
  padding-left: var(--space-4);
  color: var(--color-fg-secondary);
  margin-bottom: var(--space-4);
}

.markdown-body :deep(hr) {
  border: none;
  border-top: 1px solid var(--color-border-subtle);
  margin: var(--space-6) 0;
}
</style>
