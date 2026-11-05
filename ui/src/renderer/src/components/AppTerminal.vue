<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// A compact terminal for use inside a surface, such as a card: a fixed number of rows, as many
// columns as fit its width, on the app's terminal font and colors. It knows nothing about where
// its bytes come from: the owner writes output in and receives what the human types.
import { onBeforeUnmount, onMounted, ref } from 'vue'
import { Terminal } from '@xterm/xterm'
import { FitAddon } from '@xterm/addon-fit'
import '@xterm/xterm/css/xterm.css'
import { terminalFontReady, terminalLook } from '../terminalLook'

const props = withDefaults(
  defineProps<{
    label: string
    rows?: number
  }>(),
  { rows: 12 }
)

const emit = defineEmits<{
  data: [data: string]
  resize: [size: { rows: number; cols: number }]
}>()

const host = ref<HTMLElement | null>(null)
let term: Terminal | undefined
let fit: FitAddon | undefined
let observer: ResizeObserver | undefined
let disposed = false
// Output written before the terminal opens, which waits on the font.
const pending: (string | Uint8Array)[] = []
let wantFocus = false

function refit(): void {
  if (!term || !fit) return
  const cols = fit.proposeDimensions()?.cols
  if (!cols || (cols === term.cols && props.rows === term.rows)) return
  term.resize(cols, props.rows)
  emit('resize', { rows: term.rows, cols: term.cols })
}

async function copySelection(): Promise<void> {
  const text = term?.getSelection()
  if (text) await navigator.clipboard.writeText(text)
}

// Through Terminal.paste, so a program in bracketed paste mode receives the text as one paste.
async function pasteClipboard(): Promise<void> {
  const text = await navigator.clipboard.readText()
  if (text && term && !disposed) term.paste(text)
}

onMounted(async () => {
  await terminalFontReady()
  if (disposed || !host.value) return
  const background = getComputedStyle(host.value).getPropertyValue('--terminal-bg').trim()
  term = new Terminal({
    ...terminalLook(background || undefined),
    rows: props.rows,
    cursorStyle: 'bar',
    cursorBlink: true,
    scrollback: 2000
  })
  fit = new FitAddon()
  term.loadAddon(fit)
  term.open(host.value)
  term.textarea?.setAttribute('aria-label', props.label)
  term.onData((data) => emit('data', data))
  // The same clipboard keys as an agent tab, since Ctrl+C and Ctrl+V belong to the program.
  term.attachCustomKeyEventHandler((event) => {
    const clipboardKey = event.ctrlKey && event.shiftKey && !event.altKey && !event.metaKey
    if (!clipboardKey || (event.code !== 'KeyC' && event.code !== 'KeyV')) return true
    event.preventDefault()
    if (event.type === 'keydown') {
      if (event.code === 'KeyC') void copySelection()
      else void pasteClipboard()
    }
    return false
  })
  refit()
  emit('resize', { rows: term.rows, cols: term.cols })
  // The font wait gives up after a few seconds, and xterm measures its cell only when the font
  // option changes, so the font is set again once every face has loaded.
  void document.fonts.ready.then(() => {
    if (!term || disposed) return
    const family = term.options.fontFamily ?? ''
    term.options.fontFamily = ''
    term.options.fontFamily = family
    refit()
  })
  for (const chunk of pending) term.write(chunk)
  pending.length = 0
  if (wantFocus) term.focus()
  observer = new ResizeObserver(() => refit())
  observer.observe(host.value)
})

onBeforeUnmount(() => {
  disposed = true
  observer?.disconnect()
  term?.dispose()
})

function write(data: string | Uint8Array): void {
  if (term) term.write(data)
  else pending.push(data)
}

// Clears the screen and scrollback, for a fresh replay of the same output.
function reset(): void {
  if (term) term.reset()
  else pending.length = 0
}

function focus(): void {
  if (term) term.focus()
  else wantFocus = true
}

defineExpose({ write, reset, focus })
</script>

<template>
  <div class="app-terminal">
    <div ref="host" class="terminal-host" />
  </div>
</template>

<style scoped>
.app-terminal {
  /* Black, set apart from the card around it; the terminal reads its background from here. */
  --terminal-bg: var(--color-black);
  padding: var(--space-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  background: var(--terminal-bg);
}

.app-terminal:focus-within {
  border-color: var(--color-jade-dim);
}

.terminal-host {
  width: 100%;
  min-width: 0;
}
</style>
