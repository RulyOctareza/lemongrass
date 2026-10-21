<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { switchBranch } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitBranches } from '../../../preload/types'

// With pickOnly the menu names a branch for the caller instead of checking it out.
const props = defineProps<{
  project: ProjectRef
  pickOnly?: boolean
  disabled?: boolean
  title?: string
}>()

const emit = defineEmits<{
  pick: [name: string]
}>()

const MENU_MIN_WIDTH = 220
const MENU_MAX_HEIGHT = 320
const GAP = 4

const open = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const filterRef = ref<HTMLInputElement | null>(null)
const position = ref({ left: 0, top: 0, width: MENU_MIN_WIDTH, maxHeight: MENU_MAX_HEIGHT })
const branches = ref<GitBranches | null>(null)
const filter = ref('')
const error = ref<string | null>(null)
const switching = ref(false)

function matches(name: string): boolean {
  return name.toLowerCase().includes(filter.value.trim().toLowerCase())
}

const local = computed(() => (branches.value?.local ?? []).filter(matches))

// A remote branch that already has a local twin is reached through the local one.
const remote = computed(() => {
  const owned = new Set(props.pickOnly ? [] : (branches.value?.local ?? []))
  return (branches.value?.remote ?? []).filter(
    (name) => !owned.has(name.slice(name.indexOf('/') + 1)) && matches(name)
  )
})

// The git body scrolls and clips, so the menu is fixed to the window and opens upward when the
// trigger sits too close to the bottom edge, which is where the git panel lives.
function place(): void {
  const rect = triggerRef.value?.getBoundingClientRect()
  if (!rect) return
  const width = Math.max(rect.width, MENU_MIN_WIDTH)
  const left = Math.max(GAP, Math.min(rect.left, window.innerWidth - width - GAP))
  const below = window.innerHeight - rect.bottom - GAP * 2
  const above = rect.top - GAP * 2
  if (below >= MENU_MAX_HEIGHT || below >= above) {
    position.value = {
      left,
      top: rect.bottom + GAP,
      width,
      maxHeight: Math.min(MENU_MAX_HEIGHT, below)
    }
  } else {
    const maxHeight = Math.min(MENU_MAX_HEIGHT, above)
    position.value = { left, top: rect.top - GAP - maxHeight, width, maxHeight }
  }
}

async function show(): Promise<void> {
  filter.value = ''
  error.value = null
  branches.value = null
  place()
  open.value = true
  addListeners()
  await nextTick()
  filterRef.value?.focus()
  branches.value = await window.api.git.branches(props.project.path)
}

function hide(): void {
  open.value = false
  removeListeners()
}

function toggle(): void {
  if (open.value) hide()
  else void show()
}

async function pick(name: string, isRemote: boolean): Promise<void> {
  if (switching.value) return
  if (props.pickOnly) {
    emit('pick', name)
    hide()
    return
  }
  error.value = null
  switching.value = true
  error.value = await switchBranch(props.project, name, isRemote)
  switching.value = false
  if (!error.value) hide()
}

function pickFirst(): void {
  const first = local.value.find((name) => name !== branches.value?.current)
  if (first) void pick(first, false)
  else if (remote.value[0]) void pick(remote.value[0], true)
}

function isInside(target: EventTarget | null): boolean {
  const node = target as Node | null
  return !!node && (!!triggerRef.value?.contains(node) || !!menuRef.value?.contains(node))
}

function onPointerDown(event: PointerEvent): void {
  if (!isInside(event.target)) hide()
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') hide()
}

// A fixed menu would drift away from its trigger, so any scroll outside it closes it.
function onScroll(event: Event): void {
  if (!menuRef.value?.contains(event.target as Node)) hide()
}

function addListeners(): void {
  window.addEventListener('pointerdown', onPointerDown, true)
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('scroll', onScroll, true)
  window.addEventListener('resize', hide)
}

function removeListeners(): void {
  window.removeEventListener('pointerdown', onPointerDown, true)
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('scroll', onScroll, true)
  window.removeEventListener('resize', hide)
}

onBeforeUnmount(removeListeners)
</script>

<template>
  <button
    ref="triggerRef"
    type="button"
    class="branch-trigger"
    :class="{ open }"
    :disabled="disabled"
    :title="title"
    :aria-expanded="open"
    aria-haspopup="listbox"
    @click="toggle"
  >
    <slot />
    <svg
      class="branch-caret"
      width="9"
      height="9"
      viewBox="0 0 10 10"
      fill="none"
      stroke="currentColor"
      stroke-width="1.4"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <path d="M2 3.5L5 6.5L8 3.5" />
    </svg>
  </button>

  <Teleport to="body">
    <div
      v-if="open"
      ref="menuRef"
      class="branch-menu"
      :style="{
        left: `${position.left}px`,
        top: `${position.top}px`,
        width: `${position.width}px`,
        maxHeight: `${position.maxHeight}px`
      }"
    >
      <input
        ref="filterRef"
        v-model="filter"
        class="branch-filter"
        placeholder="Filter branches"
        aria-label="Filter branches"
        @keydown.enter="pickFirst"
      />
      <p v-if="error" class="branch-error">{{ error }}</p>
      <p v-if="!branches" class="branch-empty">Loading...</p>
      <div v-else class="branch-lists" role="listbox">
        <template v-if="local.length > 0">
          <p class="branch-heading">Local</p>
          <button
            v-for="name in local"
            :key="`local-${name}`"
            type="button"
            class="branch-row"
            :class="{ current: name === branches.current }"
            :disabled="name === branches.current || switching"
            @click="pick(name, false)"
          >
            <span class="branch-row-name">{{ name }}</span>
            <span v-if="name === branches.current" class="branch-tag">current</span>
          </button>
        </template>
        <template v-if="remote.length > 0">
          <p class="branch-heading">Remote</p>
          <button
            v-for="name in remote"
            :key="`remote-${name}`"
            type="button"
            class="branch-row"
            :disabled="switching"
            :title="pickOnly ? undefined : 'Creates a local branch that tracks it'"
            @click="pick(name, true)"
          >
            <span class="branch-row-name">{{ name }}</span>
          </button>
        </template>
        <p v-if="local.length === 0 && remote.length === 0" class="branch-empty">
          No branches match.
        </p>
      </div>
    </div>
  </Teleport>
</template>

<style scoped>
.branch-trigger {
  display: flex;
  align-items: center;
  gap: 6px;
  min-width: 0;
  padding: 3px var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.branch-trigger:hover:not(:disabled),
.branch-trigger.open {
  background: var(--color-surface-2);
}

.branch-trigger:disabled {
  cursor: default;
}

.branch-trigger:hover:not(:disabled) :slotted(.git-icon),
.branch-trigger:focus-visible:not(:disabled) :slotted(.git-icon) {
  color: var(--color-fg-accent);
}

.branch-caret {
  flex-shrink: 0;
  color: var(--color-fg-muted);
  transition: transform var(--duration-fast) var(--ease-out);
}

.branch-trigger.open .branch-caret {
  transform: rotate(180deg);
}

.branch-menu {
  position: fixed;
  z-index: 80;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
}

.branch-filter {
  box-sizing: border-box;
  flex-shrink: 0;
  width: 100%;
  padding: var(--space-1) var(--space-2);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
}

.branch-filter:focus {
  outline: none;
  border-color: var(--color-amber);
}

.branch-lists {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow-y: auto;
}

.branch-heading {
  padding: var(--space-1) var(--space-2);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.branch-row {
  display: flex;
  flex-shrink: 0;
  align-items: center;
  gap: var(--space-2);
  padding: 4px var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.branch-row:hover:not(:disabled) {
  background: var(--color-surface-2);
}

.branch-row:disabled {
  cursor: default;
}

.branch-row.current {
  color: var(--color-fg-accent);
}

.branch-row-name {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.branch-tag {
  margin-left: auto;
  color: var(--color-fg-muted);
}

.branch-empty {
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.branch-error {
  flex-shrink: 0;
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
</style>
