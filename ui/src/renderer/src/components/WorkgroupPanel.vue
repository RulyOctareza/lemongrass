<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, watch } from 'vue'
import { openApprovals, pendingFor, refreshWorkgroups, workgroupStateOf } from '../workgroups'
import {
  focusedWorkgroupId,
  isTabOpen,
  liveShellCount,
  openWorkgroup,
  type ProjectRef
} from '../workspace'
import type { WorkgroupInfo } from '../../../preload/types'

const POLL_MS = 5000

const props = defineProps<{
  project: ProjectRef
}>()

const state = computed(() => workgroupStateOf(props.project.id))
const groups = computed(() => state.value.groups)
const waiting = computed(() => pendingFor(props.project.path))
const focusedId = computed(() => focusedWorkgroupId(props.project.id))

function orphaned(group: WorkgroupInfo): boolean {
  return !isTabOpen(props.project.id, group.leaderTabId)
}

function refresh(): void {
  void refreshWorkgroups(props.project)
}

function onFocus(): void {
  refresh()
}

let timer: ReturnType<typeof setInterval> | undefined

onMounted(() => {
  refresh()
  timer = setInterval(() => {
    if (document.hasFocus()) refresh()
  }, POLL_MS)
  window.addEventListener('focus', onFocus)
})

onBeforeUnmount(() => {
  clearInterval(timer)
  window.removeEventListener('focus', onFocus)
})

watch(() => props.project.id, refresh)
// A workgroup's tabs open right after the group is created, so a change in the shell count is the cue to look again.
watch(() => liveShellCount(props.project.id), refresh)
</script>

<template>
  <div class="workgroup-panel">
    <button
      class="wg-item"
      :class="{ open: !state.folded }"
      :aria-expanded="!state.folded"
      @click="state.folded = !state.folded"
    >
      <svg
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <circle cx="7" cy="3.5" r="1.7" />
        <circle cx="3" cy="10.5" r="1.7" />
        <circle cx="11" cy="10.5" r="1.7" />
        <path d="M6 5l-2 4M8 5l2 4M4.7 10.5h4.6" />
      </svg>
      <span class="wg-label">Workgroup</span>
      <span v-if="groups.length > 0" class="wg-count">{{ groups.length }}</span>
      <span
        v-if="waiting.length > 0"
        class="wg-alert"
        role="img"
        aria-label="Action needed"
        title="A workgroup is waiting for your approval"
      >
        !
      </span>
      <svg
        class="wg-chevron"
        width="10"
        height="10"
        viewBox="0 0 10 10"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <path d="M3 2l3 3-3 3" />
      </svg>
    </button>

    <div v-if="!state.folded" class="wg-body">
      <button v-if="waiting.length > 0" class="wg-row waiting" @click="openApprovals()">
        <span class="wg-row-name">Waiting approval</span>
        <span class="wg-count">{{ waiting.length }}</span>
      </button>
      <button
        v-for="group in groups"
        :key="group.id"
        class="wg-row"
        :class="{ shown: focusedId === group.id }"
        :title="group.name"
        @click="openWorkgroup(project, group.id)"
      >
        <span class="wg-row-name">{{ group.name }}</span>
        <span v-if="orphaned(group)" class="wg-orphaned" title="The leader's tab is not open">
          Orphaned
        </span>
      </button>
      <p v-if="groups.length === 0 && waiting.length === 0" class="wg-empty">
        No workgroup is active. A leader agent can start thinkers in other tabs of this project and
        coordinate them through one shared thread. Ask an agent to set one up.
      </p>
    </div>
  </div>
</template>

<style scoped>
.workgroup-panel {
  flex-shrink: 0;
  padding: var(--space-2);
  border-top: 1px solid var(--color-border-default);
}

.wg-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.wg-item:hover {
  background: var(--color-surface-3);
}

.wg-item.open {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.wg-label {
  flex-shrink: 0;
}

.wg-count {
  flex-shrink: 0;
  min-width: 18px;
  padding: 0 6px;
  border-radius: var(--radius-pill);
  background: var(--color-amber);
  color: var(--color-black);
  font-size: 11px;
  line-height: 18px;
  text-align: center;
}

.wg-chevron {
  flex-shrink: 0;
  margin-left: auto;
  transition: transform var(--duration-fast) var(--ease-out);
}

.wg-item.open .wg-chevron {
  transform: rotate(90deg);
}

.wg-alert {
  flex-shrink: 0;
  width: 18px;
  border-radius: var(--radius-pill);
  background: var(--color-error);
  color: var(--color-black);
  font-size: 11px;
  font-weight: var(--weight-bold);
  line-height: 18px;
  text-align: center;
}

.wg-body {
  display: flex;
  flex-direction: column;
  gap: 2px;
  max-height: 55vh;
  overflow-y: auto;
  padding: var(--space-2) var(--space-1) 0;
}

.wg-empty {
  padding: 0 var(--space-2) var(--space-1);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.wg-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: var(--space-1) var(--space-3);
  background: var(--color-surface-2);
  border: none;
  border-radius: var(--radius-sm);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.wg-row:hover {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}

.wg-row.shown {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.wg-row.waiting {
  color: var(--color-fg-accent);
  font-weight: var(--weight-semibold);
}

.wg-row-name {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.wg-orphaned {
  flex-shrink: 0;
  color: var(--color-warning);
  font-size: var(--text-xs);
}
</style>
