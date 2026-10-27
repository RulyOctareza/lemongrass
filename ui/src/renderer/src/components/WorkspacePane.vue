<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'
import AppConfirmButton from './AppConfirmButton.vue'
import TabIcon from './TabIcon.vue'
import { accountLabel, accountStore } from '../accounts'
import {
  activateTab,
  addShell,
  addShellInNewPane,
  closeAllDocuments,
  closeAllTabs,
  closeDocumentsInPane,
  closeOtherTabs,
  closeTab,
  closeTabsInPane,
  docTabCount,
  focusPane,
  openInNewShell,
  openWeb,
  placeTab,
  projectRelativePath,
  tabBand,
  type ProjectRef
} from '../workspace'
import { tabKindOf, type TabContext } from '../tabKinds'
import { joinVendor, liveGroupsOf, openJoin } from '../workgroups'
import type { WorkgroupInfo } from '../../../preload/types'
import type { DropZone, EdgeZone } from '../layoutTree'
import type { WorkspaceLayoutNode, WorkspaceTab } from '../../../preload/types'

type Leaf = Extract<WorkspaceLayoutNode, { type: 'leaf' }>

const props = defineProps<{
  leaf: Leaf
  project: ProjectRef
  focused: boolean
}>()

const TAB_MIME = 'application/x-lemongrass-doc-tab'
const EDGE_RATIO = 0.25

const activeTab = computed(() => props.leaf.tabs.find((t) => t.id === props.leaf.activeTabId))

function tabContext(tab: WorkspaceTab): TabContext {
  return { project: props.project, active: tab.id === props.leaf.activeTabId }
}

const shellGroup = computed(() => {
  const active = props.leaf.tabs.find((tab) => tab.id === props.leaf.activeTabId)
  return active?.kind === 'shell' ? tabBand(props.project.id, active) : null
})

const shellAccount = computed(() => {
  const active = props.leaf.tabs.find((tab) => tab.id === props.leaf.activeTabId)
  if (active?.kind !== 'shell' || active.command !== 'claude') return null
  if (!active.account && accountStore.items.length === 0) return null
  return accountLabel(active.account)
})

const tabBar = ref<HTMLDivElement>()

function onTabBarWheel(event: WheelEvent): void {
  if (!tabBar.value || Math.abs(event.deltaY) <= Math.abs(event.deltaX)) return
  event.preventDefault()
  tabBar.value.scrollLeft += event.deltaY
}

watch(
  () => props.leaf.activeTabId,
  async (tabId) => {
    await nextTick()
    tabBar.value?.querySelector(`[data-tab-id="${tabId}"]`)?.scrollIntoView({
      inline: 'nearest',
      block: 'nearest'
    })
  },
  { immediate: true }
)

function onTabDragStart(event: DragEvent, tabId: string): void {
  if (!event.dataTransfer) return
  event.dataTransfer.effectAllowed = 'copyMove'
  event.dataTransfer.setData(TAB_MIME, JSON.stringify({ paneId: props.leaf.id, tabId }))
}

const bodyEl = ref<HTMLDivElement>()
const dropZone = ref<DropZone | null>(null)
const dropDuplicate = ref(false)

function isTabDrag(event: DragEvent): boolean {
  return event.dataTransfer?.types.includes(TAB_MIME) ?? false
}

function zoneAt(event: MouseEvent): DropZone {
  const rect = bodyEl.value!.getBoundingClientRect()
  const x = (event.clientX - rect.left) / rect.width
  const y = (event.clientY - rect.top) / rect.height
  const nearest = Math.min(x, 1 - x, y, 1 - y)
  if (nearest > EDGE_RATIO) return 'center'
  if (nearest === x) return 'left'
  if (nearest === 1 - x) return 'right'
  return nearest === y ? 'up' : 'down'
}

function onBodyDragOver(event: DragEvent): void {
  if (!isTabDrag(event)) return
  event.preventDefault()
  event.stopPropagation()
  dropZone.value = zoneAt(event)
  dropDuplicate.value = event.altKey
  if (event.dataTransfer) event.dataTransfer.dropEffect = event.altKey ? 'copy' : 'move'
}

function onBodyDragLeave(event: DragEvent): void {
  if (!bodyEl.value?.contains(event.relatedTarget as Node | null)) dropZone.value = null
}

function receiveDrop(event: DragEvent, zone: DropZone): void {
  const raw = event.dataTransfer?.getData(TAB_MIME)
  if (!raw) return
  const { paneId, tabId } = JSON.parse(raw) as { paneId: string; tabId: string }
  placeTab(props.project, paneId, tabId, props.leaf.id, zone, event.altKey)
}

function onBodyDrop(event: DragEvent): void {
  if (!isTabDrag(event)) return
  event.preventDefault()
  event.stopPropagation()
  const zone = dropZone.value ?? 'center'
  dropZone.value = null
  receiveDrop(event, zone)
}

function onTabBarDragOver(event: DragEvent): void {
  if (!isTabDrag(event)) return
  event.preventDefault()
  if (event.dataTransfer) event.dataTransfer.dropEffect = event.altKey ? 'copy' : 'move'
}

function onTabBarDrop(event: DragEvent): void {
  if (!isTabDrag(event)) return
  event.preventDefault()
  receiveDrop(event, 'center')
}

const menu = ref<{ tabId: string; x: number; y: number } | null>(null)
const menuTab = computed(() => props.leaf.tabs.find((t) => t.id === menu.value?.tabId))

const MENU_DIRECTIONS: { zone: EdgeZone; label: string }[] = [
  { zone: 'right', label: 'Right' },
  { zone: 'down', label: 'Below' },
  { zone: 'left', label: 'Left' },
  { zone: 'up', label: 'Above' }
]

const PANE_ICONS: Record<EdgeZone, { divider: string; fill: [number, number, number, number] }> = {
  right: { divider: 'M7 2v10', fill: [7, 2, 5.5, 10] },
  left: { divider: 'M7 2v10', fill: [1.5, 2, 5.5, 10] },
  up: { divider: 'M1.5 7h11', fill: [1.5, 2, 11, 5] },
  down: { divider: 'M1.5 7h11', fill: [1.5, 7, 11, 5] }
}

function openMenu(event: MouseEvent, tabId: string): void {
  menu.value = { tabId, x: event.clientX, y: event.clientY }
}

function menuPlace(zone: EdgeZone, duplicate: boolean): void {
  if (!menu.value) return
  placeTab(props.project, props.leaf.id, menu.value.tabId, props.leaf.id, zone, duplicate)
  menu.value = null
}

function menuClose(): void {
  if (!menu.value) return
  closeTab(props.project, props.leaf.id, menu.value.tabId)
  menu.value = null
}

const TAB_ICON = [
  'M2.2 4.2h9.6c.7 0 1.2.5 1.2 1.2v5.4c0 .7-.5 1.2-1.2 1.2H2.2c-.7 0-1.2-.5-1.2-1.2V5.4c0-.7.5-1.2 1.2-1.2Z',
  'M3.5 2h3.2',
  'M5.2 6.2l3.6 3.6M8.8 6.2l-3.6 3.6'
]

const DOC_ICON = [
  'M2.5 4.5v7.3c0 .7.5 1.2 1.2 1.2h5.8',
  'M5.7 2.2h4.6c.7 0 1.2.5 1.2 1.2v6.4c0 .7-.5 1.2-1.2 1.2H5.7c-.7 0-1.2-.5-1.2-1.2V3.4c0-.7.5-1.2 1.2-1.2Z',
  'M6.7 5.1l2.6 2.6M9.3 5.1L6.7 7.7'
]

const WARNING_ICON = ['M7 3v5.2', 'M7 10.8v.1']

const closeActions = computed(() => {
  const paneHasDocs = props.leaf.tabs.some((t) => t.kind === 'doc')
  return [
    {
      key: 'other',
      label: 'Close Other Tabs',
      icon: TAB_ICON,
      disabled: props.leaf.tabs.length < 2
    },
    { key: 'paneTabs', label: 'Close All Tabs In Pane', icon: TAB_ICON, disabled: false },
    { key: 'paneDocs', label: 'Close Documents In Pane', icon: DOC_ICON, disabled: !paneHasDocs },
    {
      key: 'allDocs',
      label: 'Close All Documents',
      icon: DOC_ICON,
      disabled: docTabCount(props.project.id) === 0
    },
    { key: 'allTabs', label: 'Close All Tabs', icon: TAB_ICON, disabled: false }
  ]
})

function runCloseAction(key: string): void {
  if (key === 'other') {
    if (props.leaf.activeTabId) closeOtherTabs(props.project, props.leaf.id, props.leaf.activeTabId)
  } else if (key === 'paneTabs') closeTabsInPane(props.project, props.leaf.id)
  else if (key === 'paneDocs') closeDocumentsInPane(props.project, props.leaf.id)
  else if (key === 'allDocs') closeAllDocuments(props.project)
  else closeAllTabs(props.project)
}

function menuCloseAction(key: string): void {
  if (!menu.value) return
  menu.value = null
  runCloseAction(key)
}

const copiedTabId = ref<string | null>(null)
let copiedTimer: ReturnType<typeof setTimeout> | undefined

const SHORT_TAB_ID_LENGTH = 8

async function copyTabId(tabId: string): Promise<void> {
  try {
    await navigator.clipboard.writeText(tabId.slice(0, SHORT_TAB_ID_LENGTH))
  } catch {
    return
  }
  copiedTabId.value = tabId
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copiedTabId.value = null), 1500)
}

const menuJoinGroups = computed((): WorkgroupInfo[] | null =>
  menuTab.value && joinVendor(props.project.id, menuTab.value)
    ? liveGroupsOf(props.project.id)
    : null
)

function menuJoin(group: WorkgroupInfo): void {
  const tab = menuTab.value
  if (!tab) return
  openJoin(props.project, group, tab, tabKindOf(tab).label(tab, tabContext(tab)).text)
  menu.value = null
}

function menuCopyTabId(): void {
  if (!menu.value) return
  void copyTabId(menu.value.tabId)
  menu.value = null
}

async function copyPath(tabId: string): Promise<void> {
  const tab = props.leaf.tabs.find((t) => t.id === tabId)
  if (tab?.kind !== 'doc') return
  try {
    await navigator.clipboard.writeText(projectRelativePath(tab.path))
  } catch {
    return
  }
  copiedTabId.value = tabId
  clearTimeout(copiedTimer)
  copiedTimer = setTimeout(() => (copiedTabId.value = null), 1500)
}

function menuCopyPath(): void {
  if (!menu.value) return
  void copyPath(menu.value.tabId)
  menu.value = null
}

function menuWatchBrowser(): void {
  if (!menu.value) return
  openWeb(props.project, props.leaf.id, menu.value.tabId)
  menu.value = null
}

function menuOpenInShell(): void {
  if (!menu.value) return
  openInNewShell(props.project, props.leaf.id, menu.value.tabId)
  menu.value = null
}

function dismissMenu(): void {
  menu.value = null
}

function onKeydown(event: KeyboardEvent): void {
  if (event.key === 'Escape') dismissMenu()
}

onMounted(() => {
  window.addEventListener('pointerdown', dismissMenu)
  window.addEventListener('keydown', onKeydown)
  window.addEventListener('blur', dismissMenu)
})

onBeforeUnmount(() => {
  clearTimeout(copiedTimer)
  window.removeEventListener('pointerdown', dismissMenu)
  window.removeEventListener('keydown', onKeydown)
  window.removeEventListener('blur', dismissMenu)
})
</script>

<template>
  <div
    class="workspace-pane"
    :class="{ focused }"
    @pointerdown.capture="focusPane(project, leaf.id)"
  >
    <div class="tab-header">
      <div
        ref="tabBar"
        class="tab-bar"
        @wheel="onTabBarWheel"
        @dragover="onTabBarDragOver"
        @drop="onTabBarDrop"
      >
        <div
          v-for="tab in leaf.tabs"
          :key="tab.id"
          class="tab"
          :class="{ active: tab.id === leaf.activeTabId }"
          :data-tab-id="tab.id"
          :title="tabKindOf(tab).title(tab, tabContext(tab))"
          draggable="true"
          @click="activateTab(project, leaf.id, tab.id)"
          @contextmenu.prevent="openMenu($event, tab.id)"
          @dragstart="onTabDragStart($event, tab.id)"
        >
          <template v-if="tabBand(project.id, tab)">
            <svg
              v-if="tabBand(project.id, tab)?.leader"
              class="tab-band-star"
              viewBox="0 0 14 14"
              :style="{ fill: tabBand(project.id, tab)?.color }"
            >
              <path d="M7 1l1.8 3.9 4.2.5-3.1 2.9.8 4.2L7 10.4 3.3 12.5l.8-4.2L1 5.4l4.2-.5z" />
            </svg>
            <span
              v-else
              class="tab-band"
              :style="{ background: tabBand(project.id, tab)?.color }"
            />
          </template>
          <TabIcon v-if="tabKindOf(tab).icon.length" :shapes="tabKindOf(tab).icon" />
          <span class="tab-label">
            <span v-if="tabKindOf(tab).label(tab, tabContext(tab)).prefix" class="tab-parent">
              {{ tabKindOf(tab).label(tab, tabContext(tab)).prefix }} /
            </span>
            {{ tabKindOf(tab).label(tab, tabContext(tab)).text }}
          </span>
          <span class="tab-close" @click.stop="closeTab(project, leaf.id, tab.id)">&times;</span>
        </div>
      </div>

      <div class="pane-actions">
        <button class="pane-action" title="New shell" @click="addShell(project, leaf.id)">
          <svg
            width="14"
            height="14"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
          >
            <path d="M7 2.5v9M2.5 7h9" />
          </svg>
        </button>
        <button
          class="pane-action"
          title="New shell in a pane to the right"
          @click="addShellInNewPane(project, leaf.id, 'right')"
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="1.5" y="2" width="11" height="10" rx="1.5" />
            <path d="M7 2v10" />
          </svg>
        </button>
        <button
          class="pane-action"
          title="New shell in a pane below"
          @click="addShellInNewPane(project, leaf.id, 'down')"
        >
          <svg
            width="14"
            height="14"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="1.5" y="2" width="11" height="10" rx="1.5" />
            <path d="M1.5 7h11" />
          </svg>
        </button>
        <template v-if="activeTab?.kind === 'doc'">
          <span class="action-divider" />
          <button
            class="pane-action"
            title="Read in a new shell"
            @click="openInNewShell(project, leaf.id, activeTab.id)"
          >
            <svg
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M2.5 4.5l3 2.5-3 2.5M7 10h4.5" />
            </svg>
          </button>
          <button
            class="pane-action"
            :title="copiedTabId === activeTab.id ? 'Copied' : 'Copy path'"
            @click="copyPath(activeTab.id)"
          >
            <svg
              v-if="copiedTabId === activeTab.id"
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M3 7.5l2.5 2.5L11 4.5" />
            </svg>
            <svg
              v-else
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <rect x="4.5" y="4.5" width="7" height="8" rx="1.2" />
              <path
                d="M9.5 4.5V3.2A1.2 1.2 0 0 0 8.3 2H3.2A1.2 1.2 0 0 0 2 3.2v5.1a1.2 1.2 0 0 0 1.2 1.2h1.3"
              />
            </svg>
          </button>
        </template>
      </div>
    </div>

    <div
      ref="bodyEl"
      class="pane-body"
      @dragover.capture="onBodyDragOver"
      @dragleave="onBodyDragLeave"
      @drop.capture="onBodyDrop"
    >
      <template v-for="tab in leaf.tabs" :key="tab.id">
        <component
          :is="tabKindOf(tab).body"
          v-if="tabKindOf(tab).keepAlive || tab.id === leaf.activeTabId"
          v-show="tab.id === leaf.activeTabId"
          v-bind="tabKindOf(tab).bodyProps(tab, tabContext(tab))"
        />
      </template>

      <div v-if="dropZone" class="drop-overlay" :class="dropZone">
        <span v-if="dropDuplicate" class="drop-copy">Copy</span>
      </div>
    </div>

    <div v-if="shellGroup || shellAccount" class="group-strip">
      <span v-if="shellGroup" class="strip-item">
        <span class="group-strip-band" :style="{ background: shellGroup.color }" />
        Workgroup: {{ shellGroup.groupName }}
      </span>
      <span v-if="shellAccount" class="strip-item">Account: {{ shellAccount }}</span>
    </div>

    <div
      v-if="menu && menuTab"
      class="tab-menu"
      :style="{ left: `${menu.x}px`, top: `${menu.y}px` }"
      @pointerdown.stop
    >
      <p class="menu-heading">Move to new pane</p>
      <button
        v-for="direction in MENU_DIRECTIONS"
        :key="`move-${direction.zone}`"
        class="menu-item"
        :disabled="leaf.tabs.length === 1"
        @click="menuPlace(direction.zone, false)"
      >
        <svg
          class="menu-icon"
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <rect x="1.5" y="2" width="11" height="10" rx="1.5" />
          <path :d="PANE_ICONS[direction.zone].divider" />
          <rect
            :x="PANE_ICONS[direction.zone].fill[0]"
            :y="PANE_ICONS[direction.zone].fill[1]"
            :width="PANE_ICONS[direction.zone].fill[2]"
            :height="PANE_ICONS[direction.zone].fill[3]"
            fill="currentColor"
            fill-opacity="0.35"
            stroke="none"
          />
        </svg>
        {{ direction.label }}
      </button>
      <template v-if="menuTab.kind === 'doc'">
        <p class="menu-heading">Split into new pane</p>
        <button
          v-for="direction in MENU_DIRECTIONS"
          :key="`split-${direction.zone}`"
          class="menu-item"
          @click="menuPlace(direction.zone, true)"
        >
          <svg
            class="menu-icon"
            width="14"
            height="14"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="1.5" y="2" width="11" height="10" rx="1.5" />
            <path :d="PANE_ICONS[direction.zone].divider" />
          </svg>
          {{ direction.label }}
        </button>
        <div class="menu-divider" />
        <button class="menu-item" @click="menuOpenInShell">
          <svg
            class="menu-icon"
            width="14"
            height="14"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M2.5 4.5l3 2.5-3 2.5M7 10h4.5" />
          </svg>
          Read in a new shell
        </button>
        <button class="menu-item" @click="menuCopyPath">
          <svg
            class="menu-icon"
            width="14"
            height="14"
            viewBox="0 0 14 14"
            fill="none"
            stroke="currentColor"
            stroke-width="1.2"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <rect x="4.5" y="4.5" width="7" height="8" rx="1.2" />
            <path
              d="M9.5 4.5V3.2A1.2 1.2 0 0 0 8.3 2H3.2A1.2 1.2 0 0 0 2 3.2v5.1a1.2 1.2 0 0 0 1.2 1.2h1.3"
            />
          </svg>
          Copy path
        </button>
      </template>
      <button v-else-if="menuTab.kind === 'shell'" class="menu-item" @click="menuCopyTabId">
        <svg
          class="menu-icon"
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <rect x="4.5" y="4.5" width="7" height="8" rx="1.2" />
          <path
            d="M9.5 4.5V3.2A1.2 1.2 0 0 0 8.3 2H3.2A1.2 1.2 0 0 0 2 3.2v5.1a1.2 1.2 0 0 0 1.2 1.2h1.3"
          />
        </svg>
        Copy tab id
      </button>
      <button v-if="menuTab.kind === 'shell'" class="menu-item" @click="menuWatchBrowser">
        <svg
          class="menu-icon"
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <circle cx="7" cy="7" r="5.2" />
          <path
            d="M1.8 7h10.4M7 1.8c-1.6 1.6-2.4 3.3-2.4 5.2s.8 3.6 2.4 5.2c1.6-1.6 2.4-3.3 2.4-5.2S8.6 3.4 7 1.8Z"
          />
        </svg>
        Watch browser
      </button>
      <template v-if="menuJoinGroups">
        <div class="menu-divider" />
        <p class="menu-heading">Add to workgroup</p>
        <button
          v-for="group in menuJoinGroups"
          :key="`join-${group.id}`"
          class="menu-item"
          @click="menuJoin(group)"
        >
          {{ group.name }}
        </button>
        <button v-if="!menuJoinGroups.length" class="menu-item" disabled>No live workgroup</button>
      </template>
      <div class="menu-divider" />
      <AppConfirmButton
        v-for="action in closeActions"
        :key="action.key"
        v-slot="{ armed }"
        class="menu-item"
        :disabled="action.disabled"
        @confirm="menuCloseAction(action.key)"
      >
        <svg
          class="menu-icon"
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path v-for="d in armed ? WARNING_ICON : action.icon" :key="d" :d="d" />
        </svg>
        {{ action.label }}
        <span class="menu-confirm-hint" :class="{ shown: armed }">Click again</span>
      </AppConfirmButton>
      <div class="menu-divider" />
      <button class="menu-item" @click="menuClose">
        <svg
          class="menu-icon"
          width="14"
          height="14"
          viewBox="0 0 14 14"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M3.5 3.5l7 7M10.5 3.5l-7 7" />
        </svg>
        Close tab
      </button>
    </div>
  </div>
</template>

<style scoped>
.workspace-pane {
  height: 100%;
  width: 100%;
  min-height: 0;
  min-width: 0;
  display: flex;
  flex-direction: column;
}

.tab-header {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  min-width: 0;
}

.workspace-pane.focused .tab-header {
  box-shadow: inset 0 -1px 0 var(--color-amber);
}

.pane-actions {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--space-1);
  padding: var(--space-3) var(--space-4) var(--space-2) 0;
}

.pane-action {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 26px;
  height: 26px;
  background: transparent;
  border: none;
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.pane-action:hover {
  background: var(--color-surface-1);
  color: var(--color-fg-primary);
}

.action-divider {
  width: 1px;
  height: 16px;
  margin: 0 var(--space-1);
  background: var(--color-border-default);
}

.tab-bar {
  flex: 1;
  min-width: 0;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-4) var(--space-2);
  overflow-x: auto;
  overflow-y: hidden;
}

.tab-bar::-webkit-scrollbar {
  height: 6px;
}

.tab-bar::-webkit-scrollbar-track {
  background: transparent;
}

.tab-bar::-webkit-scrollbar-thumb {
  border-width: 1px;
}

.tab {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--space-2);
  max-width: 260px;
  padding: var(--space-2) var(--space-4);
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.tab:hover {
  background: var(--color-surface-1);
}

.tab.active {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.tab.active .tab-parent {
  color: var(--color-amber-dim);
}

.tab-band {
  flex-shrink: 0;
  width: 3px;
  height: 14px;
  border-radius: var(--radius-pill);
}

.tab-band-star {
  flex-shrink: 0;
  width: 13px;
  height: 13px;
}

.group-strip {
  flex-shrink: 0;
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-1) var(--space-4);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  white-space: nowrap;
  overflow: hidden;
  text-overflow: ellipsis;
}

.strip-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.group-strip-band {
  flex-shrink: 0;
  width: 3px;
  height: 12px;
  border-radius: var(--radius-pill);
}

.tab-label {
  min-width: 3ch;
  overflow: hidden;
  white-space: nowrap;
  text-overflow: ellipsis;
}

.tab-parent {
  color: var(--color-fg-muted);
}

.tab-close {
  display: flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  width: 22px;
  height: 22px;
  font-size: var(--text-lg);
  line-height: 1;
  color: var(--color-fg-muted);
  border-radius: var(--radius-pill);
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.tab-close:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.pane-body {
  position: relative;
  flex: 1;
  min-height: 0;
  display: flex;
  flex-direction: column;
}

.drop-overlay {
  position: absolute;
  z-index: 5;
  pointer-events: none;
  background: var(--color-amber-muted);
  border: 2px solid var(--color-amber);
  border-radius: var(--radius-lg);
  display: flex;
  align-items: flex-start;
  justify-content: flex-end;
}

.drop-overlay.center {
  inset: 0;
}
.drop-overlay.left {
  inset: 0 50% 0 0;
}
.drop-overlay.right {
  inset: 0 0 0 50%;
}
.drop-overlay.up {
  inset: 0 0 50% 0;
}
.drop-overlay.down {
  inset: 50% 0 0 0;
}

.drop-copy {
  margin: var(--space-2);
  padding: var(--space-1) var(--space-2);
  background: var(--color-amber);
  color: var(--color-bg-primary);
  border-radius: var(--radius-sm);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.tab-menu {
  position: fixed;
  z-index: 50;
  min-width: 160px;
  padding: var(--space-2);
  background: var(--color-bg-elevated);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-lg, 0 8px 24px rgba(0, 0, 0, 0.4));
}

.menu-heading {
  padding: var(--space-1) var(--space-2);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.menu-item {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  width: 100%;
  padding: var(--space-1) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.menu-icon {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
}

.menu-item:hover:not(:disabled) {
  background: var(--color-surface-2);
}

.menu-item:hover:not(:disabled) .menu-icon {
  color: var(--color-fg-primary);
}

.menu-item:disabled {
  color: var(--color-fg-muted);
  cursor: default;
}

.menu-confirm-hint {
  margin-left: auto;
  padding-left: var(--space-4);
  font-size: var(--text-xs);
  visibility: hidden;
}

.menu-confirm-hint.shown {
  visibility: visible;
}

.menu-divider {
  height: 1px;
  margin: var(--space-2) 0;
  background: var(--color-border-subtle);
}
</style>
