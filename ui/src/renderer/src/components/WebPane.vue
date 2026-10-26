<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import type { WebBounds, WebState } from '../../../preload/types'

const props = defineProps<{
  shellTabId: string
}>()

const SYNC_MS = 250
// Inset of the sample points from the host's edge, so a rounded or bordered edge does not count as covered.
const SAMPLE_INSET = 4

const host = ref<HTMLDivElement>()
const state = ref<WebState | null>(null)
let attached = false
let lastBounds = ''
let pendingFrame = 0
let dragging = false

const shown = computed(() => (state.value?.open ? state.value : null))

// The view is a native surface above the DOM, so it only stays attached while nothing in the page is drawn over the host.
function covered(rect: DOMRect): boolean {
  const el = host.value
  if (!el) return true
  const left = rect.left + SAMPLE_INSET
  const top = rect.top + SAMPLE_INSET
  const right = rect.right - SAMPLE_INSET
  const bottom = rect.bottom - SAMPLE_INSET
  const points = [
    [left, top],
    [right, top],
    [left, bottom],
    [right, bottom],
    [(left + right) / 2, (top + bottom) / 2]
  ]
  return points.some(([x, y]) => {
    const hit = document.elementFromPoint(x, y)
    return hit !== null && !el.contains(hit)
  })
}

// The page may be zoomed (webFrame zoom), while the view's bounds are in window DIPs. The window is frameless, so its outer width over the page's inner width is the zoom factor.
function toBounds(rect: DOMRect): WebBounds {
  const factor = window.innerWidth > 0 ? window.outerWidth / window.innerWidth : 1
  return {
    x: Math.round(rect.left * factor),
    y: Math.round(rect.top * factor),
    width: Math.round(rect.width * factor),
    height: Math.round(rect.height * factor)
  }
}

function detach(): void {
  if (!attached) return
  attached = false
  lastBounds = ''
  void window.api.web.detach(props.shellTabId)
}

function sync(): void {
  const el = host.value
  if (!el) return
  const rect = el.getBoundingClientRect()
  const visible =
    rect.width > 0 && rect.height > 0 && !document.hidden && !dragging && !covered(rect)
  if (!visible) {
    detach()
    return
  }
  const bounds = toBounds(rect)
  const key = `${bounds.x},${bounds.y},${bounds.width},${bounds.height}`
  if (attached && key === lastBounds) return
  lastBounds = key
  if (attached) {
    void window.api.web.setBounds(props.shellTabId, bounds)
  } else {
    attached = true
    void window.api.web.attach(props.shellTabId, bounds)
  }
}

function scheduleSync(): void {
  if (pendingFrame) return
  pendingFrame = requestAnimationFrame(() => {
    pendingFrame = 0
    sync()
  })
}

function onDragStart(): void {
  dragging = true
  sync()
}

function onDragEnd(): void {
  dragging = false
  scheduleSync()
}

let resizeObserver: ResizeObserver | undefined
let mutationObserver: MutationObserver | undefined
let unsubscribe: (() => void) | undefined
let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  unsubscribe = window.api.web.onState((next) => {
    if (next.tabId === props.shellTabId) state.value = next
  })
  void window.api.web.getState(props.shellTabId).then((current) => {
    if (!state.value) state.value = current
  })
  resizeObserver = new ResizeObserver(scheduleSync)
  if (host.value) resizeObserver.observe(host.value)
  mutationObserver = new MutationObserver(scheduleSync)
  mutationObserver.observe(document.body, {
    subtree: true,
    childList: true,
    attributes: true,
    attributeFilter: ['class', 'style']
  })
  window.addEventListener('resize', scheduleSync)
  window.addEventListener('dragstart', onDragStart, true)
  window.addEventListener('dragend', onDragEnd, true)
  window.addEventListener('drop', onDragEnd, true)
  document.addEventListener('visibilitychange', scheduleSync)
  // Moves that change neither size nor DOM, such as a sibling pane resizing, are only caught here.
  timer = setInterval(sync, SYNC_MS)
  sync()
})

onBeforeUnmount(() => {
  resizeObserver?.disconnect()
  mutationObserver?.disconnect()
  unsubscribe?.()
  clearInterval(timer)
  cancelAnimationFrame(pendingFrame)
  window.removeEventListener('resize', scheduleSync)
  window.removeEventListener('dragstart', onDragStart, true)
  window.removeEventListener('dragend', onDragEnd, true)
  window.removeEventListener('drop', onDragEnd, true)
  document.removeEventListener('visibilitychange', scheduleSync)
  detach()
})
</script>

<template>
  <div class="web-pane">
    <div class="web-bar">
      <span class="web-url" :title="shown?.url">{{ shown?.url ?? 'No page open' }}</span>
      <span v-if="shown?.title" class="web-title" :title="shown.title">{{ shown.title }}</span>
      <span v-if="shown?.user" class="web-user">as {{ shown.user }}</span>
      <span class="web-watch">Watch only</span>
    </div>
    <div ref="host" class="web-host">
      <p v-if="!shown" class="web-empty">
        No page open yet. The browser appears here when an agent in that shell tab opens a page.
      </p>
    </div>
  </div>
</template>

<style scoped>
.web-pane {
  display: flex;
  flex-direction: column;
  height: 100%;
  min-height: 0;
}

.web-bar {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-4);
  border-bottom: 1px solid var(--color-border-subtle);
  background: var(--color-surface-1);
  font-size: var(--text-xs);
  white-space: nowrap;
}

.web-url {
  flex: 0 1 50%;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
}

.web-title {
  flex: 1 1 0;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  color: var(--color-fg-secondary);
}

.web-user {
  flex-shrink: 0;
  padding: 0 var(--space-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-pill);
  color: var(--color-fg-accent);
}

.web-watch {
  flex-shrink: 0;
  margin-left: auto;
  color: var(--color-fg-muted);
}

.web-host {
  display: flex;
  flex: 1;
  align-items: center;
  justify-content: center;
  min-height: 0;
  background: var(--color-surface-0);
}

.web-empty {
  max-width: 320px;
  margin: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  text-align: center;
}
</style>
