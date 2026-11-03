<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppContextMenu, { type MenuItem } from '../AppContextMenu.vue'
import AppEmptyState from '../AppEmptyState.vue'
import AppInput from '../AppInput.vue'
import AppMenuButton from '../AppMenuButton.vue'
import TabIcon from '../TabIcon.vue'
import SearchRows from '../treeSearch/SearchRows.vue'
import TreeRow from '../treeSearch/TreeRow.vue'
import TreeSearchBar from '../treeSearch/TreeSearchBar.vue'
import type { AppInputHandle } from '../appInput'
import { ageNow } from '../../ageClock'
import { formatAge } from '../../relativeAge'
import { fileIconOf, folderIconOf, type FileIcon } from '../../fileIcons'
import { gitStateOf } from '../../git'
import {
  attachExplorer,
  baseName,
  collapseAll,
  detachExplorer,
  expandDir,
  explorerOf,
  parentOf,
  revealPath,
  subscribeExplorerChanges,
  toggleDir
} from '../../explorerStore'
import {
  GIT_MARKS,
  clearClipboard,
  explorerClipboard,
  formatSize,
  gitMarksByPath,
  opErrorText,
  putOnClipboard
} from '../../explorerOps'
import {
  setDensity,
  sidebarSettings,
  toggleColumn,
  toggleHideIgnored
} from '../../explorerSettings'
import { useTreeSearch } from '../../useTreeSearch'
import { activeFilePath, openProjectFile, retargetFileTabs, type ProjectRef } from '../../workspace'
import type { SearchNode, SearchRow } from '../../treeSearch'
import type { IconShape } from '../../tabKinds'
import type { GitFileGroup, ProjectEntry, ProjectOpError } from '../../../../preload/types'

const props = defineProps<{
  project: ProjectRef
}>()

type Row =
  | { kind: 'entry'; entry: ProjectEntry; depth: number }
  | { kind: 'create'; dir: string; type: 'dir' | 'file'; depth: number }

const DRAG_MIME = 'application/x-lemongrass-project-path'
const EXPAND_ON_HOVER_MS = 700

const ICONS = {
  newFile: ['M8 1.5H3v11h4', 'M8 1.5 10.5 4v3', 'M8 1.5V4h2.5', 'M10.5 9v4M8.5 11h4'],
  newFolder: ['M7 11.5H1.5v-8h4l1.5 2h5.5v2', 'M10.5 9v4M8.5 11h4'],
  collapse: ['M2.5 2.5h9v9h-9z', 'M5 7h4'],
  view: ['M2 4h10M2 10h10', 'M5 2.5v3M9 8.5v3'],
  check: ['M3 7.5l2.5 2.5L11 4.5'],
  open: ['M3 1.5h5l3 3v8H3z', 'M8 1.5v3h3'],
  rename: ['M9.5 1.5 12.5 4.5 5 12H2V9z'],
  cut: [
    'M3.5 12a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3ZM10.5 12a1.5 1.5 0 1 1 0-3 1.5 1.5 0 0 1 0 3Z',
    'M4.6 9.3 10 2M9.4 9.3 4 2'
  ],
  copy: ['M4.5 4.5h7v8h-7z', 'M9.5 4.5V2H2v8h2.5'],
  paste: ['M4.5 2.5H3v10h8v-10H9.5', 'M5 1.5h4v2H5z'],
  duplicate: ['M4.5 4.5h7v8h-7z', 'M9.5 4.5V2H2v8h2.5', 'M8 6.5v4M6 8.5h4'],
  path: [
    'M6 8a2.5 2.5 0 0 0 3.5 0l2-2a2.5 2.5 0 0 0-3.5-3.5l-.7.7',
    'M8 6a2.5 2.5 0 0 0-3.5 0l-2 2a2.5 2.5 0 0 0 3.5 3.5l.7-.7'
  ],
  reveal: ['M1.5 3.5h4l1.5 2h5.5v6h-11z', 'M6 8.5h4M8.5 7l1.5 1.5L8.5 10'],
  trash: ['M2.5 3.5h9', 'M5.5 3.5V2h3v1.5', 'M3.5 3.5l.7 9h5.6l.7-9']
} as const

function shapesOf(paths: readonly string[]): IconShape[] {
  return paths.map((d) => ({ tag: 'path', attrs: { d } }))
}

const state = computed(() => explorerOf(props.project.id))
const view = computed(() => sidebarSettings.explorer)
const projectName = computed(() => baseName(props.project.path.replace(/[\\/]+$/, '')))
const activeFile = computed(() => activeFilePath(props.project.id))

const panel = ref<HTMLElement | null>(null)
const opError = ref<string | null>(null)
let opErrorTimer: ReturnType<typeof setTimeout> | undefined

function showError(error: ProjectOpError): void {
  opError.value = opErrorText(error)
  clearTimeout(opErrorTimer)
  opErrorTimer = setTimeout(() => (opError.value = null), 5000)
}

// Watches follow the project shown: the old project's are dropped before the new one lists.
let attachedProject: ProjectRef | null = null

function attach(project: ProjectRef): void {
  if (attachedProject && attachedProject.path !== project.path) detachExplorer(attachedProject)
  attachedProject = project
  void attachExplorer(project)
}

onMounted(() => {
  subscribeExplorerChanges()
  attach(props.project)
})

watch(
  () => props.project.path,
  () => {
    creating.value = null
    renaming.value = null
    if (search.open.value) search.close()
    attach(props.project)
  }
)

onBeforeUnmount(() => {
  if (attachedProject) detachExplorer(attachedProject)
  clearTimeout(opErrorTimer)
  clearTimeout(hoverTimer)
})

const creating = ref<{ dir: string; type: 'dir' | 'file' } | null>(null)
const renaming = ref<string | null>(null)
const draft = ref('')
const draftError = ref<string | null>(null)
const draftInput = ref<AppInputHandle | null>(null)

const rows = computed((): Row[] => {
  const out: Row[] = []
  const pending = creating.value
  const walk = (dir: string, depth: number): void => {
    if (pending && pending.dir === dir) {
      out.push({ kind: 'create', dir, type: pending.type, depth })
    }
    for (const entry of state.value.listings[dir] ?? []) {
      if (entry.ignored && view.value.hideIgnored) continue
      out.push({ kind: 'entry', entry, depth })
      if (entry.type === 'dir' && state.value.expanded[entry.path]) walk(entry.path, depth + 1)
    }
  }
  walk('', 0)
  return out
})

const entryRows = computed(() =>
  rows.value.filter((row): row is Extract<Row, { kind: 'entry' }> => row.kind === 'entry')
)

const gitMarks = computed(() =>
  view.value.columns.git
    ? gitMarksByPath(gitStateOf(props.project.id).status?.files ?? [], state.value.gitPrefix)
    : new Map<string, GitFileGroup>()
)

function gitMark(path: string): (typeof GIT_MARKS)[GitFileGroup] | null {
  const group = gitMarks.value.get(path)
  return group ? GIT_MARKS[group] : null
}

function iconOf(entry: { name: string; path: string; type: 'dir' | 'file' }): FileIcon {
  return entry.type === 'dir'
    ? folderIconOf(entry.name, !!state.value.expanded[entry.path])
    : fileIconOf(entry.name)
}

function modifiedLabel(ms: number): string {
  return formatAge(new Date(ms).toISOString(), ageNow.value)
}

const rowEls = new Map<string, InstanceType<typeof TreeRow>>()

function bindRow(path: string, el: unknown): void {
  if (el) rowEls.set(path, el as InstanceType<typeof TreeRow>)
  else rowEls.delete(path)
}

function select(path: string | null, focus = true): void {
  state.value.selected = path
  if (focus && path !== null) void nextTick(() => rowEls.get(path)?.focus())
}

function activate(entry: ProjectEntry): void {
  select(entry.path)
  if (entry.type === 'dir') toggleDir(props.project, entry.path)
  else openProjectFile(props.project, entry.path)
}

function targetDir(entry: ProjectEntry | null): string {
  if (!entry) return ''
  return entry.type === 'dir' ? entry.path : parentOf(entry.path)
}

function selectedEntry(): ProjectEntry | null {
  const path = state.value.selected
  return entryRows.value.find((row) => row.entry.path === path)?.entry ?? null
}

function bindDraft(el: unknown): void {
  draftInput.value = (el as AppInputHandle | null) ?? null
}

function focusDraft(selectStem: boolean): void {
  void nextTick(() => {
    draftInput.value?.focus()
    if (!selectStem) return
    const input = (draftInput.value as unknown as { el?: HTMLInputElement })?.el
    const dot = draft.value.lastIndexOf('.')
    if (input) input.setSelectionRange(0, dot > 0 ? dot : draft.value.length)
  })
}

async function startCreate(
  type: 'dir' | 'file',
  entry: ProjectEntry | null = selectedEntry()
): Promise<void> {
  renaming.value = null
  const dir = targetDir(entry)
  if (dir !== '') await expandDir(props.project, dir)
  creating.value = { dir, type }
  draft.value = ''
  draftError.value = null
  focusDraft(false)
}

function startRename(entry: ProjectEntry): void {
  creating.value = null
  renaming.value = entry.path
  draft.value = entry.name
  draftError.value = null
  focusDraft(true)
}

function cancelDraft(): void {
  const path = renaming.value
  creating.value = null
  renaming.value = null
  draftError.value = null
  if (path) select(path)
  else panel.value?.focus()
}

let committing = false

async function commitDraft(): Promise<void> {
  if (committing) return
  const name = draft.value.trim()
  const pending = creating.value
  const from = renaming.value
  if (!name || (from && name === baseName(from))) {
    cancelDraft()
    return
  }
  committing = true
  try {
    if (pending) {
      const api = window.api.projectFiles
      const result =
        pending.type === 'dir'
          ? await api.createFolder(props.project.path, pending.dir, name)
          : await api.createFile(props.project.path, pending.dir, name)
      if ('error' in result) {
        draftError.value = opErrorText(result.error)
        focusDraft(false)
        return
      }
      creating.value = null
      await revealPath(props.project, result.path)
      select(result.path)
      if (pending.type === 'file') openProjectFile(props.project, result.path)
    } else if (from) {
      const result = await window.api.projectFiles.rename(props.project.path, from, name)
      if ('error' in result) {
        draftError.value = opErrorText(result.error)
        focusDraft(false)
        return
      }
      renaming.value = null
      retargetFileTabs(props.project, from, result.path)
      select(result.path)
    }
    draftError.value = null
  } finally {
    committing = false
  }
}

function onDraftKeydown(event: KeyboardEvent): void {
  event.stopPropagation()
  if (event.key === 'Enter') void commitDraft()
  else if (event.key === 'Escape') cancelDraft()
}

// Leaving the field keeps a typed name and drops an empty one, like the file managers do.
function onDraftBlur(): void {
  if (draftError.value) return
  void commitDraft()
}

function cutOrCopy(mode: 'cut' | 'copy', entry: ProjectEntry): void {
  putOnClipboard(props.project.path, mode, [entry.path])
}

const canPaste = computed(
  () => explorerClipboard.projectPath === props.project.path && explorerClipboard.paths.length > 0
)

async function paste(destDir: string): Promise<void> {
  if (!canPaste.value) return
  const paths = [...explorerClipboard.paths]
  if (explorerClipboard.mode === 'cut') {
    const result = await window.api.projectFiles.move(props.project.path, paths, destDir)
    const done = 'error' in result ? result.done : result.paths
    // The move reports the new paths in the order it was given the old ones.
    done.forEach((to, index) => retargetFileTabs(props.project, paths[index], to))
    if ('error' in result) showError(result.error)
    else clearClipboard()
    if (done.length) await revealPath(props.project, done[0])
    return
  }
  const result = await window.api.projectFiles.copy(props.project.path, paths, destDir)
  if ('error' in result) showError(result.error)
  const done = 'error' in result ? result.done : result.paths
  if (done.length) await revealPath(props.project, done[0])
}

async function duplicate(entry: ProjectEntry): Promise<void> {
  const result = await window.api.projectFiles.duplicate(props.project.path, entry.path)
  if ('error' in result) {
    showError(result.error)
    return
  }
  await revealPath(props.project, result.path)
  select(result.path)
}

async function trash(entry: ProjectEntry): Promise<void> {
  const result = await window.api.projectFiles.trash(props.project.path, [entry.path])
  if ('error' in result) showError(result.error)
  if (state.value.selected === entry.path) select(null)
  panel.value?.focus()
}

function absolutePath(path: string): string {
  const separator = props.project.path.includes('\\') ? '\\' : '/'
  const root = props.project.path.replace(/[\\/]+$/, '')
  return path ? `${root}${separator}${path.split('/').join(separator)}` : root
}

async function copyText(text: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(text)
  } catch {
    return
  }
}

const menu = ref<{ x: number; y: number; entry: ProjectEntry | null } | null>(null)

function openMenu(event: MouseEvent, entry: ProjectEntry | null): void {
  if (entry) select(entry.path)
  menu.value = { x: event.clientX, y: event.clientY, entry }
}

const menuItems = computed((): MenuItem[] => {
  const entry = menu.value?.entry ?? null
  const items: MenuItem[] = []
  if (entry?.type === 'file') items.push({ key: 'open', label: 'Open', icon: ICONS.open })
  items.push(
    { key: 'newFile', label: 'New file', icon: ICONS.newFile },
    { key: 'newFolder', label: 'New folder', icon: ICONS.newFolder }
  )
  if (entry) {
    items.push(
      { key: 'rename', label: 'Rename', icon: ICONS.rename, title: 'F2' },
      { key: 'duplicate', label: 'Duplicate', icon: ICONS.duplicate },
      { key: 'cut', label: 'Cut', icon: ICONS.cut },
      { key: 'copy', label: 'Copy', icon: ICONS.copy }
    )
  }
  items.push({ key: 'paste', label: 'Paste', icon: ICONS.paste, disabled: !canPaste.value })
  items.push(
    { key: 'copyPath', label: 'Copy path', icon: ICONS.path },
    { key: 'copyRelative', label: 'Copy relative path', icon: ICONS.path, disabled: !entry },
    { key: 'reveal', label: 'Reveal in file manager', icon: ICONS.reveal }
  )
  if (entry) items.push({ key: 'trash', label: 'Delete', icon: ICONS.trash, confirm: true })
  return items
})

function onMenuSelect(key: string): void {
  const entry = menu.value?.entry ?? null
  menu.value = null
  const path = entry?.path ?? ''
  if (key === 'open' && entry) openProjectFile(props.project, entry.path)
  else if (key === 'newFile') void startCreate('file', entry)
  else if (key === 'newFolder') void startCreate('dir', entry)
  else if (key === 'rename' && entry) startRename(entry)
  else if (key === 'duplicate' && entry) void duplicate(entry)
  else if ((key === 'cut' || key === 'copy') && entry) cutOrCopy(key, entry)
  else if (key === 'paste') void paste(targetDir(entry))
  else if (key === 'copyPath') void copyText(absolutePath(path))
  else if (key === 'copyRelative') void copyText(path)
  else if (key === 'reveal') window.api.projectFiles.reveal(props.project.path, path)
  else if (key === 'trash' && entry) void trash(entry)
}

const viewItems = computed((): MenuItem[] => {
  const on = (value: boolean): readonly string[] => (value ? ICONS.check : [])
  const { columns, density, hideIgnored } = view.value
  return [
    { key: 'size', label: 'Size', icon: on(columns.size) },
    { key: 'modified', label: 'Modified', icon: on(columns.modified) },
    { key: 'git', label: 'Git status', icon: on(columns.git) },
    { key: 'hideIgnored', label: 'Hide ignored files', icon: on(hideIgnored) },
    { key: 'compact', label: 'Compact rows', icon: on(density === 'compact') },
    { key: 'comfortable', label: 'Comfortable rows', icon: on(density === 'comfortable') }
  ]
})

function onViewSelect(key: string): void {
  if (key === 'size' || key === 'modified' || key === 'git') toggleColumn(key)
  else if (key === 'hideIgnored') toggleHideIgnored()
  else if (key === 'compact' || key === 'comfortable') setDensity(key)
}

const dragPath = ref<string | null>(null)
const dropDir = ref<string | null>(null)
let hoverTimer: ReturnType<typeof setTimeout> | undefined
let hoverDir: string | null = null

function onDragStart(event: DragEvent, entry: ProjectEntry): void {
  if (!event.dataTransfer) return
  dragPath.value = entry.path
  event.dataTransfer.setData(DRAG_MIME, entry.path)
  event.dataTransfer.effectAllowed = 'copyMove'
}

function canDrop(dest: string, copy: boolean): boolean {
  const source = dragPath.value
  if (source === null) return false
  if (dest === source || dest.startsWith(`${source}/`)) return false
  return copy || parentOf(source) !== dest
}

function onDragOver(event: DragEvent, entry: ProjectEntry | null): void {
  if (!event.dataTransfer?.types.includes(DRAG_MIME)) return
  event.stopPropagation()
  const dest = targetDir(entry)
  if (!canDrop(dest, event.altKey)) {
    dropDir.value = null
    return
  }
  event.preventDefault()
  event.dataTransfer.dropEffect = event.altKey ? 'copy' : 'move'
  dropDir.value = dest
  // Holding over a closed folder opens it, so a drop can reach a folder deeper in the tree.
  const closed = entry?.type === 'dir' && !state.value.expanded[entry.path] ? entry.path : null
  if (closed !== hoverDir) {
    clearTimeout(hoverTimer)
    hoverDir = closed
    if (closed)
      hoverTimer = setTimeout(() => void expandDir(props.project, closed), EXPAND_ON_HOVER_MS)
  }
}

function endDrag(): void {
  dragPath.value = null
  dropDir.value = null
  hoverDir = null
  clearTimeout(hoverTimer)
}

async function onDrop(event: DragEvent, entry: ProjectEntry | null): Promise<void> {
  const source = event.dataTransfer?.getData(DRAG_MIME)
  if (!source) return
  event.preventDefault()
  event.stopPropagation()
  const dest = targetDir(entry)
  const copy = event.altKey
  const allowed = canDrop(dest, copy)
  endDrag()
  if (!allowed) return
  const api = window.api.projectFiles
  const result = copy
    ? await api.copy(props.project.path, [source], dest)
    : await api.move(props.project.path, [source], dest)
  if ('error' in result) {
    showError(result.error)
    return
  }
  if (!copy) retargetFileTabs(props.project, source, result.paths[0])
  if (dest !== '') await expandDir(props.project, dest)
  select(result.paths[0])
}

function onScrollDragLeave(event: DragEvent): void {
  if (!(event.currentTarget as HTMLElement).contains(event.relatedTarget as Node | null)) {
    dropDir.value = null
  }
}

const searchIgnored = new Set<string>()

const search = useTreeSearch({
  panel,
  debounceMs: 120,
  candidates: async (query) => {
    const result = await window.api.projectFiles.search(props.project.path, query)
    const items = result.paths.filter((hit) => !(hit.ignored && view.value.hideIgnored))
    searchIgnored.clear()
    for (const hit of items) if (hit.ignored) searchIgnored.add(hit.path)
    return { items, truncated: result.truncated }
  },
  open: openResult
})

// A file opens and stays found; a folder leaves the search and is shown open in the tree.
async function openResult(row: SearchRow): Promise<void> {
  const path = row.node.path
  if (row.node.type === 'file') {
    state.value.selected = path
    openProjectFile(props.project, path)
    return
  }
  search.close()
  await revealPath(props.project, path)
  await expandDir(props.project, path)
  select(path)
}

watch(
  () => view.value.hideIgnored,
  () => search.refresh()
)

function searchIcon(node: SearchNode): FileIcon {
  return node.type === 'dir' ? folderIconOf(node.name, true) : fileIconOf(node.name)
}

function typedIntoField(event: KeyboardEvent): boolean {
  return !!(event.target as HTMLElement | null)?.closest('input, textarea')
}

function moveSelection(delta: number): void {
  const list = entryRows.value
  if (!list.length) return
  const at = list.findIndex((row) => row.entry.path === state.value.selected)
  const next = at === -1 ? (delta > 0 ? 0 : list.length - 1) : at + delta
  const row = list[Math.max(0, Math.min(list.length - 1, next))]
  select(row.entry.path)
}

function onKeydown(event: KeyboardEvent): void {
  if (typedIntoField(event) || search.active.value) {
    search.onPanelKeydown(event)
    return
  }
  const entry = selectedEntry()
  const mod = event.ctrlKey || event.metaKey
  let handled = true
  if (event.key === 'ArrowDown') moveSelection(1)
  else if (event.key === 'ArrowUp') moveSelection(-1)
  else if (event.key === 'ArrowRight' && entry?.type === 'dir') {
    if (state.value.expanded[entry.path]) moveSelection(1)
    else void expandDir(props.project, entry.path)
  } else if (event.key === 'ArrowLeft' && entry) {
    if (entry.type === 'dir' && state.value.expanded[entry.path])
      toggleDir(props.project, entry.path)
    else if (parentOf(entry.path) !== '') select(parentOf(entry.path))
  } else if (event.key === 'Enter' && entry) activate(entry)
  else if (event.key === 'F2' && entry) startRename(entry)
  else if (mod && event.key.toLowerCase() === 'c' && entry) cutOrCopy('copy', entry)
  else if (mod && event.key.toLowerCase() === 'x' && entry) cutOrCopy('cut', entry)
  else if (mod && event.key.toLowerCase() === 'v') void paste(targetDir(entry))
  else handled = false
  if (handled) {
    event.preventDefault()
    return
  }
  search.onPanelKeydown(event)
}

function isCut(path: string): boolean {
  return (
    explorerClipboard.mode === 'cut' &&
    explorerClipboard.projectPath === props.project.path &&
    explorerClipboard.paths.includes(path)
  )
}

function rowKey(row: Row): string {
  return row.kind === 'entry' ? row.entry.path : `create:${row.dir}`
}
</script>

<template>
  <div
    ref="panel"
    class="file-explorer"
    tabindex="0"
    role="tree"
    aria-label="Project files"
    @keydown="onKeydown"
  >
    <div class="explorer-head">
      <span class="head-label" :title="project.path">{{ projectName }}</span>
      <button
        class="tool"
        type="button"
        title="New file"
        aria-label="New file"
        @click="startCreate('file')"
      >
        <TabIcon :shapes="shapesOf(ICONS.newFile)" />
      </button>
      <button
        class="tool"
        type="button"
        title="New folder"
        aria-label="New folder"
        @click="startCreate('dir')"
      >
        <TabIcon :shapes="shapesOf(ICONS.newFolder)" />
      </button>
      <button
        class="tool"
        type="button"
        title="Collapse all"
        aria-label="Collapse all"
        @click="collapseAll(project)"
      >
        <TabIcon :shapes="shapesOf(ICONS.collapse)" />
      </button>
      <AppMenuButton :items="viewItems" label="View options" @select="onViewSelect">
        <TabIcon :shapes="shapesOf(ICONS.view)" />
      </AppMenuButton>
    </div>

    <div
      class="explorer-scroll"
      :class="{ 'drop-root': dropDir === '' && dragPath !== null }"
      @contextmenu.prevent="openMenu($event, null)"
      @dragover="onDragOver($event, null)"
      @dragleave="onScrollDragLeave"
      @drop="onDrop($event, null)"
    >
      <TreeSearchBar v-if="search.open.value" :search="search" placeholder="Search files" />
      <p v-if="opError" class="op-error" role="alert" @click="opError = null">{{ opError }}</p>

      <SearchRows
        v-if="search.active.value"
        :search="search"
        :icon-of="searchIcon"
        :muted-of="(node) => searchIgnored.has(node.path)"
        :active-path="activeFile"
        :density="view.density"
      />
      <template v-else>
        <AppEmptyState
          v-if="state.rootMissing"
          size="sm"
          title="This project folder cannot be listed."
        />
        <AppEmptyState v-else-if="!state.listings['']" size="sm" loading title="Listing files..." />
        <AppEmptyState
          v-else-if="!rows.length"
          size="sm"
          title="This folder is empty."
          detail="Right-click to add a file."
        />
        <template v-for="row in rows" :key="rowKey(row)">
          <div
            v-if="row.kind === 'create' || renaming === row.entry.path"
            class="edit-row"
            :style="{ paddingLeft: `${row.depth * 14 + 25}px` }"
            @contextmenu.stop
          >
            <div class="edit-line">
              <TabIcon
                class="edit-icon"
                :shapes="
                  row.kind === 'create'
                    ? iconOf({ name: draft, path: '', type: row.type }).shapes
                    : iconOf(row.entry).shapes
                "
                :style="{
                  color:
                    row.kind === 'create'
                      ? iconOf({ name: draft, path: '', type: row.type }).color
                      : iconOf(row.entry).color
                }"
              />
              <AppInput
                :ref="bindDraft"
                v-model="draft"
                class="edit-field"
                size="sm"
                :aria-label="
                  row.kind === 'create'
                    ? row.type === 'dir'
                      ? 'New folder name'
                      : 'New file name'
                    : 'New name'
                "
                @keydown="onDraftKeydown"
                @input="draftError = null"
                @blur="onDraftBlur"
              />
            </div>
            <p v-if="draftError" class="edit-error" role="alert">{{ draftError }}</p>
          </div>
          <TreeRow
            v-else
            :ref="(el) => bindRow(row.entry.path, el)"
            :depth="row.depth"
            :name="row.entry.name"
            :icon="iconOf(row.entry)"
            :dir="row.entry.type === 'dir'"
            :expanded="!!state.expanded[row.entry.path]"
            :active="row.entry.path === activeFile"
            :selected="row.entry.path === state.selected"
            :muted="row.entry.ignored || isCut(row.entry.path)"
            :drop-target="row.entry.type === 'dir' && dropDir === row.entry.path"
            :density="view.density"
            :title="row.entry.symlink ? `${row.entry.name} (link)` : undefined"
            draggable="true"
            @click="activate(row.entry)"
            @contextmenu.prevent.stop="openMenu($event, row.entry)"
            @dragstart="onDragStart($event, row.entry)"
            @dragover="onDragOver($event, row.entry)"
            @drop="onDrop($event, row.entry)"
            @dragend="endDrag"
          >
            <span v-if="view.columns.size" class="col size">{{
              row.entry.type === 'file' && row.entry.sizeBytes !== undefined
                ? formatSize(row.entry.sizeBytes)
                : ''
            }}</span>
            <span v-if="view.columns.modified" class="col modified">{{
              modifiedLabel(row.entry.modifiedMs)
            }}</span>
            <span
              v-if="view.columns.git"
              class="col git"
              :style="{ color: gitMark(row.entry.path)?.color }"
              :title="gitMark(row.entry.path)?.title"
              >{{ gitMark(row.entry.path)?.letter ?? '' }}</span
            >
          </TreeRow>
        </template>
      </template>
    </div>

    <Teleport to="body">
      <AppContextMenu
        v-if="menu"
        :x="menu.x"
        :y="menu.y"
        :items="menuItems"
        @select="onMenuSelect"
        @close="menu = null"
      />
    </Teleport>
  </div>
</template>

<style scoped>
.file-explorer {
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
  outline: none;
}

.explorer-head {
  display: flex;
  align-items: center;
  gap: 2px;
  padding: var(--space-2) var(--space-3) var(--space-1) var(--space-3);
}

.head-label {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.tool {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  padding: 3px;
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-muted);
  cursor: pointer;
}

.tool:hover,
.tool:focus-visible {
  color: var(--color-fg-accent);
}

.tool:focus-visible {
  outline: 1px solid var(--color-amber);
}

.explorer-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  display: flex;
  flex-direction: column;
  gap: 1px;
  padding: var(--space-1) var(--space-2) var(--space-4);
  border-radius: var(--radius-md);
}

.explorer-scroll.drop-root {
  box-shadow: inset 0 0 0 1px var(--color-amber);
}

.op-error {
  margin: 0 0 var(--space-1);
  padding: var(--space-1) var(--space-2);
  border-radius: var(--radius-md);
  background: var(--color-error-muted);
  color: var(--color-error);
  font-size: var(--text-xs);
  cursor: pointer;
}

.edit-row {
  display: flex;
  flex-direction: column;
  gap: 2px;
  padding-right: var(--space-2);
}

.edit-line {
  display: flex;
  align-items: center;
  gap: 6px;
}

.edit-icon {
  flex-shrink: 0;
}

.edit-line .edit-field {
  flex: 1;
  min-width: 0;
}

.edit-error {
  margin: 0;
  color: var(--color-error);
  font-size: var(--text-xs);
}

.col {
  flex-shrink: 0;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  font-variant-numeric: tabular-nums;
  text-align: right;
  white-space: nowrap;
}

.col.size {
  width: 52px;
}

.col.modified {
  width: 84px;
  overflow: hidden;
  text-overflow: ellipsis;
}

.col.git {
  width: 10px;
  font-family: var(--font-mono);
  font-weight: var(--weight-semibold);
  text-align: center;
}
</style>
