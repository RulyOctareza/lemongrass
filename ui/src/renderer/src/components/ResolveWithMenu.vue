<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import AppConfirmButton from './AppConfirmButton.vue'
import { resolveTargetsOf } from '../reviewTargets'
import { glowTab } from '../tabGlow'
import type { ProjectRef } from '../workspace'
import type { TabBusy } from '../../../preload/types'

// The tab list for a hand-over. It has no trigger of its own: the caller opens it under or above
// an anchor, such as the whole split button, with toggle(anchor).
const props = defineProps<{
  project: ProjectRef
  // Set while a hand-over runs, so a row cannot send twice.
  sending?: boolean
}>()

const emit = defineEmits<{ pick: [tabId: string] }>()

const MENU_WIDTH = 260
const MENU_MAX_HEIGHT = 320
const GAP = 4
// Each poll runs lgrassd once, so this is as often as the picker can afford.
const BUSY_POLL_MS = 2500

const open = ref(false)
let anchor: HTMLElement | null = null
const menuRef = ref<HTMLElement | null>(null)
// Opening upward fixes the bottom edge, not the top: the menu is often far shorter than its
// maximum height, and a top computed from the maximum left it floating above its anchor.
const position = ref<{ left: number; top?: number; bottom?: number; maxHeight: number }>({
  left: 0,
  top: 0,
  maxHeight: MENU_MAX_HEIGHT
})
const busy = ref<Record<string, TabBusy>>({})
let pollTimer: ReturnType<typeof setInterval> | null = null

const targets = computed(() =>
  resolveTargetsOf(props.project.id).map((target) => {
    const state = busy.value[target.tabId]
    return { ...target, reason: state?.busy ? (state.reason ?? 'Busy') : null }
  })
)

// The git panel sits at the bottom of its column and its body scrolls, so the menu is teleported,
// fixed to the window from the anchor's rectangle, and opens upward when there is more room above.
function place(): void {
  const rect = anchor?.getBoundingClientRect()
  if (!rect) return
  const left = Math.max(GAP, Math.min(rect.left, window.innerWidth - MENU_WIDTH - GAP))
  const below = window.innerHeight - rect.bottom - GAP * 2
  const above = rect.top - GAP * 2
  if (below >= MENU_MAX_HEIGHT || below >= above) {
    position.value = { left, top: rect.bottom + GAP, maxHeight: Math.min(MENU_MAX_HEIGHT, below) }
  } else {
    position.value = {
      left,
      bottom: window.innerHeight - rect.top + GAP,
      maxHeight: Math.min(MENU_MAX_HEIGHT, above)
    }
  }
}

async function pollBusy(): Promise<void> {
  try {
    busy.value = await window.api.tabSessions.states(props.project.path)
  } catch {
    busy.value = {}
  }
}

async function show(): Promise<void> {
  if (!anchor) return
  open.value = true
  place()
  void pollBusy()
  pollTimer = setInterval(pollBusy, BUSY_POLL_MS)
  window.addEventListener('pointerdown', onPointerDown, true)
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', hide)
  await nextTick()
  place()
}

function hide(): void {
  open.value = false
  glowTab(null)
  if (pollTimer) clearInterval(pollTimer)
  pollTimer = null
  window.removeEventListener('pointerdown', onPointerDown, true)
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', hide)
}

function toggle(at: HTMLElement | null): void {
  if (open.value) {
    hide()
    return
  }
  anchor = at
  void show()
}

// A press on the anchor counts as inside, so the anchor's own click can close the menu.
function isInside(target: EventTarget | null): boolean {
  const node = target as Node | null
  return !!node && (!!anchor?.contains(node) || !!menuRef.value?.contains(node))
}

function onPointerDown(event: PointerEvent): void {
  if (!isInside(event.target)) hide()
}

// An armed row stops its own Escape, so the first Escape drops the confirm and the next closes.
function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') hide()
}

// A fixed menu would drift away from its trigger, so any scroll outside it closes it.
function onScroll(event: Event): void {
  if (!menuRef.value?.contains(event.target as Node)) hide()
}

function pick(tabId: string): void {
  emit('pick', tabId)
}

defineExpose({ toggle, hide })

onBeforeUnmount(hide)
</script>

<template>
  <Teleport to="body">
    <div
      v-if="open"
      ref="menuRef"
      class="resolve-menu"
      role="menu"
      :style="{
        left: `${position.left}px`,
        top: position.top === undefined ? undefined : `${position.top}px`,
        bottom: position.bottom === undefined ? undefined : `${position.bottom}px`,
        width: `${MENU_WIDTH}px`,
        maxHeight: `${position.maxHeight}px`
      }"
    >
      <p class="menu-head">Hand the conflict to</p>
      <p v-if="targets.length === 0" class="menu-empty">
        No Claude or Codex tab is open in this project.
      </p>
      <ul v-else class="menu-list">
        <li
          v-for="target in targets"
          :key="target.tabId"
          @mouseenter="glowTab(target.tabId)"
          @mouseleave="glowTab(null)"
        >
          <AppConfirmButton
            :key="target.tabId"
            tone="jade"
            class="menu-row"
            role="menuitem"
            :disabled="!!target.reason || sending"
            :title="target.reason ?? `Hand the conflict to ${target.label}`"
            confirm-label="Click again to hand the conflict over"
            @confirm="pick(target.tabId)"
          >
            <template #default="{ armed }">
              <span class="row-label">{{ target.label }}</span>
              <span v-if="target.reason" class="row-note">{{ target.reason }}</span>
              <span v-else class="row-hint" :class="{ shown: armed }">Click again</span>
            </template>
          </AppConfirmButton>
        </li>
      </ul>
    </div>
  </Teleport>
</template>

<style scoped>
.resolve-menu {
  position: fixed;
  z-index: 80;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2);
  overflow-y: auto;
  background: var(--layer-float);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
  font-size: var(--text-xs);
}

.menu-head {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
  font-weight: var(--weight-semibold);
}

.menu-empty {
  padding: var(--space-1) var(--space-2);
  color: var(--color-fg-muted);
}

.menu-list {
  list-style: none;
  margin: 0;
  padding: 0;
}

.menu-row {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-1) var(--space-2);
  background: transparent;
  border: 1px solid transparent;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.menu-row:hover:not(:disabled) {
  background: var(--layer-float-hover);
}

.menu-row:disabled {
  color: var(--color-fg-disabled);
  cursor: default;
}

.row-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.row-note {
  flex-shrink: 0;
  max-width: 50%;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-disabled);
}

/* Always laid out, so the row does not change width when it arms. */
.row-hint {
  flex-shrink: 0;
  visibility: hidden;
}

.row-hint.shown {
  visibility: visible;
}
</style>
