<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// Not AppSwitcher: each row carries a second line and its own delete action, which a button row of
// AppSwitcher cannot hold. The dropdown renders in body for the same reason AppSwitcher's does.
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppInput from './AppInput.vue'
import AppConfirmButton from './AppConfirmButton.vue'
import AppIcon from './AppIcon.vue'
import type { AppInputHandle } from './appInput'
import { placeFloat, placementStyle } from '../floatPlace'
import { useProjectDelete } from '../projectDelete'
import type { Project } from '../../../preload/types'

const props = defineProps<{
  projects: Project[]
  activeProjectId: string | null
}>()

const emit = defineEmits<{
  select: [id: string]
  add: []
  deleted: [id: string]
}>()

const GAP = 8

const open = ref(false)
const rootRef = ref<HTMLElement | null>(null)
const triggerRef = ref<HTMLElement | null>(null)
const dropdownRef = ref<HTMLElement | null>(null)
const listRef = ref<HTMLElement | null>(null)
const searchRef = ref<AppInputHandle | null>(null)
const query = ref('')
const highlighted = ref(0)
const dropdownStyle = ref<Record<string, string>>({})
const placed = ref(false)
// Live shells are reread on every opening.
const { refreshLiveShells, refusal: deleteRefusal, remove: removeProject } = useProjectDelete()
const home = ref('')
let naturalHeight = 0

// The row's second line is only the folder the project sits in, with ~ for home: the name is already
// on the first line, and the parent is what tells two same-named projects apart.
function parentLabel(path: string): string {
  const cut = Math.max(path.lastIndexOf('/'), path.lastIndexOf('\\'))
  const parent = cut > 0 ? path.slice(0, cut) : path
  const h = home.value
  if (!h) return parent
  if (parent === h) return '~'
  return parent.startsWith(h + '/') || parent.startsWith(h + '\\')
    ? '~' + parent.slice(h.length)
    : parent
}

const activeLabel = computed(
  () => props.projects.find((p) => p.id === props.activeProjectId)?.name ?? 'No project'
)

const shown = computed(() => {
  const q = query.value.trim().toLowerCase()
  if (!q) return props.projects
  return props.projects.filter(
    (p) => p.name.toLowerCase().includes(q) || p.path.toLowerCase().includes(q)
  )
})

watch(shown, () => {
  highlighted.value = Math.min(highlighted.value, Math.max(shown.value.length - 1, 0))
})

function place(): void {
  const trigger = triggerRef.value
  const dropdown = dropdownRef.value
  if (!trigger || !dropdown) return
  const p = placeFloat(trigger.getBoundingClientRect(), dropdown.offsetWidth, naturalHeight, GAP)
  dropdownStyle.value = placementStyle(p)
  placed.value = true
}

function listen(on: boolean): void {
  const method = on ? 'addEventListener' : 'removeEventListener'
  window[method]('resize', place)
  document[method]('scroll', place, true)
}

watch(open, (isOpen) => listen(isOpen))

async function toggleOpen(): Promise<void> {
  if (open.value) {
    open.value = false
    return
  }
  query.value = ''
  const active = props.projects.findIndex((p) => p.id === props.activeProjectId)
  highlighted.value = Math.max(active, 0)
  placed.value = false
  dropdownStyle.value = {}
  open.value = true
  void refreshLiveShells()
  await nextTick()
  naturalHeight = dropdownRef.value?.offsetHeight ?? 0
  place()
  searchRef.value?.focus()
  scrollHighlightedIntoView()
}

function pick(id: string): void {
  emit('select', id)
  open.value = false
}

function pickAdd(): void {
  emit('add')
  open.value = false
}

function scrollHighlightedIntoView(): void {
  const row = listRef.value?.querySelectorAll('.project-row')[highlighted.value]
  row?.scrollIntoView({ block: 'nearest' })
}

function move(step: number): void {
  if (shown.value.length === 0) return
  highlighted.value = (highlighted.value + step + shown.value.length) % shown.value.length
  void nextTick(scrollHighlightedIntoView)
}

function pickHighlighted(): void {
  const project = shown.value[highlighted.value]
  if (project) pick(project.id)
}

async function remove(id: string): Promise<void> {
  if (await removeProject(id)) emit('deleted', id)
}

function onDocumentClickOutside(event: MouseEvent): void {
  if (!open.value) return
  const target = event.target as Node
  if (rootRef.value?.contains(target) || dropdownRef.value?.contains(target)) return
  open.value = false
}

onMounted(async () => {
  document.addEventListener('click', onDocumentClickOutside, true)
  home.value = await window.api.projects.home()
})
onBeforeUnmount(() => {
  document.removeEventListener('click', onDocumentClickOutside, true)
  listen(false)
})
</script>

<template>
  <div ref="rootRef" class="project-switcher">
    <button
      ref="triggerRef"
      type="button"
      class="trigger"
      :aria-expanded="open"
      @click="toggleOpen"
    >
      <span class="trigger-label">{{ activeLabel }}</span>
      <AppIcon class="lg-trigger-caret" name="chevronDown" :size="9" />
    </button>

    <Teleport to="body">
      <div
        v-if="open"
        ref="dropdownRef"
        class="dropdown lg-float"
        :class="{ placed }"
        :style="dropdownStyle"
      >
        <AppInput
          ref="searchRef"
          v-model="query"
          class="search"
          icon="search"
          spellcheck="false"
          placeholder="Search projects"
          @keydown.down.prevent="move(1)"
          @keydown.up.prevent="move(-1)"
          @keydown.enter.prevent="pickHighlighted"
          @keydown.escape="open = false"
        />
        <div ref="listRef" class="project-list" role="listbox">
          <div
            v-for="(project, i) in shown"
            :key="project.id"
            class="project-row"
            role="option"
            :aria-selected="project.id === activeProjectId"
            :class="{
              'is-active': project.id === activeProjectId,
              'is-highlighted': i === highlighted
            }"
            @mouseenter="highlighted = i"
          >
            <button
              type="button"
              class="project-pick"
              :title="project.path"
              @click="pick(project.id)"
            >
              <span class="project-name">
                <span class="active-dot" aria-hidden="true" />
                <span class="name-text">{{ project.name }}</span>
              </span>
              <span class="project-path">{{ parentLabel(project.path) }}</span>
            </button>
            <span
              v-if="deleteRefusal(project.id)"
              class="delete-slot"
              :title="deleteRefusal(project.id)"
            >
              <button type="button" class="delete" disabled :aria-label="deleteRefusal(project.id)">
                <AppIcon name="trash" :size="16" />
              </button>
            </span>
            <AppConfirmButton
              v-else
              :key="project.id"
              class="delete"
              title="Delete project"
              aria-label="Delete project"
              confirm-label="Click again to delete. The folder is not touched, and lemongrass keeps its data for 30 days."
              @confirm="remove(project.id)"
            >
              <AppIcon name="trash" :size="16" />
            </AppConfirmButton>
          </div>
          <p v-if="projects.length === 0" class="lg-menu-empty">No projects yet.</p>
          <p v-else-if="shown.length === 0" class="lg-menu-empty">No match.</p>
        </div>
        <button type="button" class="add-item" @click="pickAdd">
          <span class="icon">+</span>
          <span>Add project</span>
        </button>
      </div>
    </Teleport>
  </div>
</template>

<style scoped>
.project-switcher {
  position: relative;
}

.trigger {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-1) var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.trigger:hover,
.trigger[aria-expanded='true'] {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.trigger .lg-trigger-caret {
  margin-left: 0;
}

.trigger-label {
  max-width: 220px;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Above the app modal, below the app's closing overlay. Hidden until placed, so it never flashes at
   the window corner. */
.dropdown {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 950;
  width: 340px;
  display: flex;
  flex-direction: column;
  overflow: hidden;
  visibility: hidden;
}

.dropdown.placed {
  visibility: visible;
}

.search {
  margin: var(--space-2) var(--space-2) 0;
}

.project-list {
  flex: 1 1 auto;
  min-height: 0;
  max-height: 360px;
  overflow-y: auto;
  padding: var(--space-2);
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.project-row {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding-right: var(--space-2);
  border-radius: var(--radius-md);
  transition: background var(--duration-fast) var(--ease-out);
}

.project-row.is-active {
  background: color-mix(in srgb, var(--color-amber-muted) 45%, transparent);
}

.project-row.is-highlighted {
  background: var(--layer-float-hover);
}

.project-pick {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  align-items: flex-start;
  gap: 1px;
  padding: 6px var(--space-2);
  background: transparent;
  border: none;
  font-family: var(--font-body);
  text-align: left;
  cursor: pointer;
}

.project-name {
  max-width: 100%;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  font-weight: var(--weight-medium);
}

.name-text {
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

/* Every row keeps the dot's slot, so names line up whichever row is active. */
.active-dot {
  flex-shrink: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
}

.project-row.is-active .active-dot {
  background: var(--color-amber);
}

.project-path {
  max-width: 100%;
  overflow: hidden;
  padding-left: calc(6px + var(--space-2));
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  text-overflow: ellipsis;
  white-space: nowrap;
}

.delete-slot {
  display: inline-flex;
}

/* Shown only on the row under the pointer or keyboard; hidden keeps its space, so the path's width
   never jumps. Padding gives the confirm's progress bar and armed background room around the glyph. */
.delete {
  flex-shrink: 0;
  display: inline-flex;
  padding: 4px;
  background: none;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  visibility: hidden;
  cursor: pointer;
  transition: color var(--duration-fast) var(--ease-out);
}

.project-row.is-highlighted .delete,
.project-row:focus-within .delete {
  visibility: visible;
}

.delete:hover:not(:disabled),
.delete:focus-visible:not(:disabled) {
  color: var(--color-error);
}

.project-row .delete:disabled {
  opacity: 0.4;
  cursor: default;
}

.add-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  border-top: 1px solid var(--color-border-default);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.add-item:hover {
  background: var(--color-amber);
  color: var(--color-black);
}

.icon {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 16px;
  font-size: var(--text-sm);
  line-height: 1;
}
</style>
