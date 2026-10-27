<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { deleteBranch, gitStateOf, switchBranch, updateBranch } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitBranches, GitLocalBranch } from '../../../preload/types'
import AppContextMenu, { type MenuItem } from './AppContextMenu.vue'
import GitCounts from './GitCounts.vue'

// blockedReason names why the current branch cannot be left now, such as a stopped rebase, which
// blocks merge, checkout and deleting the checked-out branch. Update only fetches or fast-forwards,
// so it stays available.
const props = defineProps<{
  project: ProjectRef
  blockedReason: string | null
}>()

// Main deletes the checked-out branch by moving to this one first, so it cannot be deleted while
// it is checked out.
const FALLBACK_BRANCH = 'main'

const MENU_MIN_WIDTH = 220
const MENU_MAX_HEIGHT = 320
const GAP = 4

interface Selected {
  name: string
  remote: boolean
  local: GitLocalBranch | null
  x: number
  y: number
}

const state = computed(() => gitStateOf(props.project.id))
const open = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const filterRef = ref<HTMLInputElement | null>(null)
const position = ref({ left: 0, top: 0, width: MENU_MIN_WIDTH, maxHeight: MENU_MAX_HEIGHT })
const branches = ref<GitBranches | null>(null)
const filter = ref('')
const error = ref<string | null>(null)
const notice = ref<string | null>(null)
const selected = ref<Selected | null>(null)
// Commits of the selected local branch that no other branch or remote has: undefined while
// loading, null when git could not tell, in which case the delete itself reports why.
const onlyHere = ref<number | null | undefined>(undefined)
let onlyHereRequest = 0

function matches(name: string): boolean {
  return name.toLowerCase().includes(filter.value.trim().toLowerCase())
}

const local = computed(() => (branches.value?.local ?? []).filter((branch) => matches(branch.name)))

// A remote branch that already has a local twin is reached through the local one.
const remote = computed(() => {
  const owned = new Set((branches.value?.local ?? []).map((branch) => branch.name))
  return (branches.value?.remote ?? []).filter(
    (name) => !owned.has(name.slice(name.indexOf('/') + 1)) && matches(name)
  )
})

const current = computed(() => branches.value?.current ?? null)

function deleteBlockedReason(target: Selected): string | null {
  const operation = state.value.status?.operation
  if (operation) return `Continue or abort the ${operation} first`
  if (target.name === current.value) {
    if (target.name === FALLBACK_BRANCH) {
      return `Deleting falls back to ${FALLBACK_BRANCH}. Check out another branch to delete it`
    }
    if (props.blockedReason) return props.blockedReason
    if (!branches.value?.local.some((branch) => branch.name === FALLBACK_BRANCH)) {
      return `There is no ${FALLBACK_BRANCH} branch to fall back to`
    }
  }
  if (onlyHere.value === undefined) return 'Checking for commits found only on this branch'
  if (onlyHere.value) {
    const commits = onlyHere.value === 1 ? '1 commit exists' : `${onlyHere.value} commits exist`
    return `${commits} only on ${target.name}. Merge or push them first`
  }
  return null
}

const actions = computed((): MenuItem[] => {
  const target = selected.value
  if (!target) return []
  const busy = state.value.busy ? 'An operation is running' : null
  const items: MenuItem[] = []
  const noUpstream = !target.remote && !target.local?.upstream ? 'No upstream' : null
  items.push({
    key: 'update',
    label: 'Update',
    disabled: !!(busy ?? noUpstream),
    title:
      busy ??
      noUpstream ??
      (target.remote
        ? `Fetch ${target.name}`
        : `Fetch ${target.local?.upstream} and fast-forward ${target.name}`)
  })
  if (target.remote || target.name !== current.value) {
    const mergeBlocked = busy ?? props.blockedReason ?? (current.value ? null : 'HEAD is detached')
    items.push({
      key: 'merge',
      label: current.value ? `Merge into ${current.value}` : 'Merge',
      disabled: !!mergeBlocked,
      title: mergeBlocked ?? `Merge ${target.name} into ${current.value}`
    })
    const checkoutBlocked = busy ?? props.blockedReason
    items.push({
      key: 'checkout',
      label: 'Checkout',
      disabled: !!checkoutBlocked,
      title:
        checkoutBlocked ??
        (target.remote ? 'Creates a local branch that tracks it' : `Check out ${target.name}`)
    })
  }
  if (!target.remote) {
    const deleteBlocked = busy ?? deleteBlockedReason(target)
    items.push({
      key: 'delete',
      label: 'Delete',
      confirm: true,
      disabled: !!deleteBlocked,
      title:
        deleteBlocked ??
        (target.name === current.value
          ? `Switch to ${FALLBACK_BRANCH} with any uncommitted changes, then delete ${target.name}`
          : `Delete ${target.name}`)
    })
  }
  return items
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

async function loadBranches(): Promise<void> {
  branches.value = await window.api.git.branches(props.project.path)
}

async function show(): Promise<void> {
  filter.value = ''
  error.value = null
  notice.value = null
  branches.value = null
  closeActions()
  place()
  open.value = true
  addListeners()
  await nextTick()
  filterRef.value?.focus()
  await loadBranches()
}

function hide(): void {
  open.value = false
  closeActions()
  removeListeners()
}

function toggle(): void {
  if (open.value) hide()
  else void show()
}

// The action menu opens beside the row, inside the branch menu so its clicks count as inside.
function openActions(row: HTMLElement, name: string, isRemote: boolean): void {
  const rect = row.getBoundingClientRect()
  onlyHere.value = undefined
  if (!isRemote) void loadOnlyHere(name)
  selected.value = {
    name,
    remote: isRemote,
    local: isRemote ? null : (branches.value?.local.find((branch) => branch.name === name) ?? null),
    x: rect.right + GAP,
    y: rect.top
  }
}

async function loadOnlyHere(name: string): Promise<void> {
  const request = ++onlyHereRequest
  const count = await window.api.git.commitsOnlyOn(props.project.path, name)
  if (request === onlyHereRequest) onlyHere.value = count
}

function closeActions(): void {
  selected.value = null
}

async function act(key: string): Promise<void> {
  const target = selected.value
  if (!target) return
  error.value = null
  notice.value = null
  if (key === 'merge') {
    state.value.mergeSource = target.name
    hide()
  } else if (key === 'checkout') {
    closeActions()
    error.value = await switchBranch(props.project, target.name, target.remote)
    if (!error.value) hide()
  } else if (key === 'update') {
    closeActions()
    const updated = await updateBranch(props.project, target.name, target.remote)
    error.value = updated.error
    notice.value = updated.notice
    if (state.value.diverged) hide()
    else await loadBranches()
  } else if (key === 'delete') {
    closeActions()
    error.value = await deleteBranch(props.project, target.name)
    await loadBranches()
  }
}

function openFirst(): void {
  const row = menuRef.value?.querySelector<HTMLElement>('.branch-row')
  row?.click()
}

function isInside(target: EventTarget | null): boolean {
  const node = target as Node | null
  return !!node && (!!triggerRef.value?.contains(node) || !!menuRef.value?.contains(node))
}

function onPointerDown(event: PointerEvent): void {
  if (!isInside(event.target)) hide()
}

// Esc closes the action menu first, then the branch menu.
function onKeydown(event: KeyboardEvent): void {
  if (event.key !== 'Escape') return
  if (selected.value) closeActions()
  else hide()
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
    :disabled="state.busy !== null"
    title="Branches"
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
        @keydown.enter="openFirst"
      />
      <p v-if="error" class="branch-error">{{ error }}</p>
      <p v-if="notice" class="branch-notice">{{ notice }}</p>
      <p v-if="!branches" class="branch-empty">Loading...</p>
      <div v-else class="branch-lists" role="listbox">
        <template v-if="local.length > 0">
          <p class="branch-heading">Local</p>
          <button
            v-for="branch in local"
            :key="`local-${branch.name}`"
            type="button"
            class="branch-row"
            :class="{
              current: branch.name === branches.current,
              selected: !selected?.remote && selected?.name === branch.name
            }"
            @click="openActions($event.currentTarget as HTMLElement, branch.name, false)"
          >
            <span class="branch-row-name">{{ branch.name }}</span>
            <span v-if="branch.name === branches.current" class="branch-tag">current</span>
            <GitCounts :ahead="branch.ahead" :behind="branch.behind" />
          </button>
        </template>
        <template v-if="remote.length > 0">
          <p class="branch-heading">Remote</p>
          <button
            v-for="name in remote"
            :key="`remote-${name}`"
            type="button"
            class="branch-row"
            :class="{ selected: selected?.remote && selected.name === name }"
            @click="openActions($event.currentTarget as HTMLElement, name, true)"
          >
            <span class="branch-row-name">{{ name }}</span>
          </button>
        </template>
        <p v-if="local.length === 0 && remote.length === 0" class="branch-empty">
          No branches match.
        </p>
      </div>
      <AppContextMenu
        v-if="selected"
        :x="selected.x"
        :y="selected.y"
        :items="actions"
        @select="act"
        @close="closeActions"
      />
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

.branch-row.selected {
  background: var(--color-surface-2);
}

.branch-row-name {
  min-width: 0;
  margin-right: auto;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.branch-tag {
  color: var(--color-fg-muted);
}

.branch-notice {
  flex-shrink: 0;
  padding: var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
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
