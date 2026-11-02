<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, ref } from 'vue'
import { deleteBranch, gitStateOf, switchBranch, updateBranch } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitBranches, GitLocalBranch } from '../../../preload/types'
import type { MenuAnchor } from '../menuPlacement'
import AppContextMenu, { type MenuItem } from './AppContextMenu.vue'
import GitCounts from './GitCounts.vue'
import AppInput from './AppInput.vue'
import type { AppInputHandle } from './appInput'

// blockedReason names why the current branch cannot be left now, such as a stopped rebase, which
// blocks merge, checkout, new branch and deleting the checked-out branch. Update only fetches or
// fast-forwards, so it stays available. The checked-out branch has no Update here, since the
// panel's Fetch updates it.
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
  anchor: MenuAnchor
}

const state = computed(() => gitStateOf(props.project.id))
const open = ref(false)
const triggerRef = ref<HTMLElement | null>(null)
const menuRef = ref<HTMLElement | null>(null)
const filterRef = ref<AppInputHandle | null>(null)
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

// The current branch leads, the rest keep main's alphabetical order.
const local = computed(() =>
  (branches.value?.local ?? [])
    .filter((branch) => matches(branch.name))
    .sort((a, b) => Number(b.name === current.value) - Number(a.name === current.value))
)

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
  if (target.remote || target.name !== current.value) {
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
  // Creating switches to the new branch, carrying changes like a checkout.
  const newBlocked = busy ?? props.blockedReason
  items.push({
    key: 'new',
    label: 'New branch',
    disabled: !!newBlocked,
    title: newBlocked ?? `Create a branch from ${target.name} and switch to it`
  })
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
    // Beside the row, on its left when the right has no room, its top on the row's top or its
    // bottom on the row's bottom.
    anchor: {
      x: { start: rect.right + GAP, end: rect.left - GAP },
      y: { start: rect.top, end: rect.bottom }
    }
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
  } else if (key === 'new') {
    state.value.branchBase = target.name
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
  const row = menuRef.value?.querySelector<HTMLElement>('.branch-lists .lg-menu-row')
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
    class="branch-trigger lg-field-trigger lg-field-trigger--sm"
    :disabled="state.busy !== null"
    title="Branches"
    :aria-expanded="open"
    aria-haspopup="listbox"
    @click="toggle"
  >
    <slot />
    <svg
      class="lg-trigger-caret"
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
      class="branch-menu lg-float lg-menu--sm"
      :style="{
        left: `${position.left}px`,
        top: `${position.top}px`,
        width: `${position.width}px`,
        maxHeight: `${position.maxHeight}px`
      }"
    >
      <AppInput
        ref="filterRef"
        v-model="filter"
        class="branch-filter"
        size="sm"
        icon="search"
        placeholder="Filter branches"
        aria-label="Filter branches"
        @keydown.enter="openFirst"
      />
      <p v-if="error" class="branch-error">{{ error }}</p>
      <p v-if="notice" class="branch-notice">{{ notice }}</p>
      <p v-if="!branches" class="lg-menu-empty">Loading...</p>
      <div v-else class="branch-lists" role="listbox">
        <template v-if="local.length > 0">
          <p class="lg-menu-heading">Local</p>
          <button
            v-for="branch in local"
            :key="`local-${branch.name}`"
            type="button"
            class="lg-menu-row"
            :class="{
              'is-active': branch.name === branches.current,
              'is-open': !selected?.remote && selected?.name === branch.name
            }"
            @click="openActions($event.currentTarget as HTMLElement, branch.name, false)"
          >
            <span class="lg-menu-label">{{ branch.name }}</span>
            <span v-if="branch.name === branches.current" class="lg-menu-hint">current</span>
            <GitCounts :ahead="branch.ahead" :behind="branch.behind" />
          </button>
        </template>
        <template v-if="remote.length > 0">
          <p class="lg-menu-heading">Remote</p>
          <button
            v-for="name in remote"
            :key="`remote-${name}`"
            type="button"
            class="lg-menu-row"
            :class="{ 'is-open': selected?.remote && selected.name === name }"
            @click="openActions($event.currentTarget as HTMLElement, name, true)"
          >
            <span class="lg-menu-label">{{ name }}</span>
          </button>
        </template>
        <p v-if="local.length === 0 && remote.length === 0" class="lg-menu-empty">
          No branches match.
        </p>
      </div>
      <AppContextMenu
        v-if="selected"
        :anchor="selected.anchor"
        :items="actions"
        @select="act"
        @close="closeActions"
      />
    </div>
  </Teleport>
</template>

<style scoped>
.branch-trigger {
  flex-shrink: 0;
  gap: 6px;
}

.branch-trigger:hover:not(:disabled) :slotted(.git-icon),
.branch-trigger:focus-visible:not(:disabled) :slotted(.git-icon) {
  color: var(--color-fg-accent);
}

.branch-menu {
  position: fixed;
  z-index: 80;
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: var(--space-2);
}

.branch-filter {
  flex-shrink: 0;
}

.branch-lists {
  display: flex;
  flex-direction: column;
  min-height: 0;
  overflow-y: auto;
}

/* GitCounts sits after the hint, so the label alone pushes the rest to the right. */
.branch-lists .lg-menu-label {
  margin-right: auto;
}

.branch-lists .lg-menu-hint {
  margin-left: 0;
}

.branch-notice {
  flex-shrink: 0;
  padding: var(--space-2);
  background: var(--layer-float-hover);
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
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
