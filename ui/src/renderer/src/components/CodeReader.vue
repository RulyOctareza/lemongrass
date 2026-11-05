<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'
import { Compartment, EditorState } from '@codemirror/state'
import { EditorView, lineNumbers } from '@codemirror/view'
import { loadLanguage } from '../languageFor'
import { readerLook } from '../readerHighlight'

// Text shown read-only in CodeMirror with syntax colors. CodeMirror draws only the lines in view,
// so a file near the 2 MB read limit costs no more to show than a short one.
const props = defineProps<{
  content: string
  path: string
}>()

const host = ref<HTMLDivElement>()
const language = new Compartment()
let view: EditorView | undefined

// A grammar arrives after the text is already shown. An answer for a path the reader has since
// left is dropped, so a slow import never colors another file.
async function applyLanguage(path: string): Promise<void> {
  const extension = await loadLanguage(path).catch(() => null)
  if (!view || path !== props.path) return
  view.dispatch({ effects: language.reconfigure(extension ?? []) })
}

onMounted(() => {
  view = new EditorView({
    parent: host.value!,
    state: EditorState.create({
      doc: props.content,
      extensions: [
        EditorState.readOnly.of(true),
        EditorView.editable.of(false),
        EditorState.tabSize.of(4),
        lineNumbers(),
        readerLook,
        language.of([])
      ]
    })
  })
  void applyLanguage(props.path)
})

// The file is reread every time its tab is shown, so the same text comes back often; replacing
// the document only on a real change keeps the scroll position.
watch(
  () => props.content,
  (content) => {
    if (!view || view.state.doc.toString() === content) return
    const top = view.scrollDOM.scrollTop
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: content } })
    view.requestMeasure({ read: () => {}, write: () => view && (view.scrollDOM.scrollTop = top) })
  }
)

watch(
  () => props.path,
  (path) => {
    view?.dispatch({ effects: language.reconfigure([]) })
    void applyLanguage(path)
  }
)

onBeforeUnmount(() => {
  view?.destroy()
  view = undefined
})
</script>

<template>
  <div ref="host" class="code-reader" />
</template>

<style scoped>
.code-reader {
  flex: 1;
  min-height: 0;
  display: flex;
}

.code-reader :deep(.cm-editor) {
  height: 100%;
}
</style>
