<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted, watch } from 'vue'
import AppButton from './AppButton.vue'
import AppConfirmButton from './AppConfirmButton.vue'
import { ageNow } from '../ageClock'
import { askState } from '../asks'
import { formatAge } from '../relativeAge'
import { vaultSession } from '../vaultSession'
import {
  center,
  loadProjects,
  loadWaitingAsks,
  storedRows,
  waitingRows,
  type CenterRow
} from '../notifications'

const waiting = computed(() => waitingRows())
const stored = computed(() => storedRows())
const hasUnread = computed(() => center.items.some((item) => !item.read))

function markAllRead(): void {
  void window.api.notifications.markRead()
}

function clearAll(): void {
  void window.api.notifications.clear()
}

function age(row: CenterRow): string {
  return formatAge(new Date(row.at).toISOString(), ageNow.value)
}

onMounted(() => {
  loadProjects()
  void loadWaitingAsks()
})

watch(
  () => [askState.version, askState.count.waiting, vaultSession.unlocked],
  () => void loadWaitingAsks()
)
</script>

<template>
  <div class="center" role="dialog" aria-label="Notifications">
    <div class="center-head">
      <span class="center-title">Notifications</span>
      <AppButton v-if="hasUnread" @click="markAllRead"> Mark all read </AppButton>
      <AppConfirmButton
        v-if="center.items.length > 0"
        variant="ghost"
        confirm-label="Clear all?"
        @confirm="clearAll"
      >
        Clear
      </AppConfirmButton>
    </div>

    <div class="center-body">
      <template v-if="waiting.length > 0">
        <div class="section-label">Waiting for you</div>
        <button
          v-for="row in waiting"
          :key="row.key"
          type="button"
          class="row waiting"
          @click="row.open()"
        >
          <span class="row-text">{{ row.sentence }}</span>
          <span class="row-age">{{ age(row) }}</span>
        </button>
      </template>

      <template v-if="stored.length > 0">
        <div class="section-label">Activity</div>
        <button
          v-for="row in stored"
          :key="row.key"
          type="button"
          class="row"
          :class="{ unread: row.unread }"
          @click="row.open()"
        >
          <span v-if="row.unread" class="dot" aria-label="Unread" />
          <span class="row-main">
            <span v-if="row.title" class="row-title">{{ row.title }}</span>
            <span class="row-text">{{ row.sentence }}</span>
          </span>
          <span class="row-age">{{ age(row) }}</span>
        </button>
      </template>

      <p v-if="waiting.length === 0 && stored.length === 0" class="empty">
        Nothing new. Decisions, agent events and app events show up in this list.
      </p>
    </div>
  </div>
</template>

<style scoped>
.center {
  position: absolute;
  top: calc(100% + var(--space-1));
  right: 0;
  z-index: 90;
  width: 400px;
  max-height: 70vh;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
  box-shadow: var(--shadow-xl);
  overflow: hidden;
  -webkit-app-region: no-drag;
  cursor: default;
}

.center-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  padding: var(--space-3) var(--space-3) var(--space-2) var(--space-4);
  border-bottom: 1px solid var(--color-border-subtle);
}

.center-title {
  flex: 1;
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
}

.center-body {
  overflow-y: auto;
  padding: var(--space-2);
}

.section-label {
  padding: var(--space-2) var(--space-2) var(--space-1);
  color: var(--color-fg-muted);
  font-size: 11px;
  font-weight: var(--weight-semibold);
  letter-spacing: 0.04em;
  text-transform: uppercase;
}

.row {
  position: relative;
  width: 100%;
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-2) var(--space-2) var(--space-2) var(--space-4);
  background: transparent;
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  line-height: 1.45;
  text-align: left;
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.row:hover {
  background: var(--color-surface-2);
}

.row.unread {
  color: var(--color-fg-primary);
}

.row.waiting {
  color: var(--color-fg-primary);
  background: var(--color-amber-muted);
}

.row.waiting::before {
  content: '';
  position: absolute;
  left: var(--space-1);
  top: var(--space-2);
  bottom: var(--space-2);
  width: 3px;
  border-radius: var(--radius-pill);
  background: var(--color-amber);
}

.row.waiting:hover {
  background: var(--color-surface-3);
}

.dot {
  position: absolute;
  left: 6px;
  top: calc(var(--space-2) + 5px);
  width: 6px;
  height: 6px;
  border-radius: var(--radius-pill);
  background: var(--color-amber);
}

.row-main {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 2px;
}

.row-title {
  font-weight: var(--weight-semibold);
}

.row-text {
  flex: 1;
  min-width: 0;
  overflow-wrap: anywhere;
}

.row-age {
  flex-shrink: 0;
  color: var(--color-fg-muted);
  white-space: nowrap;
}

.empty {
  margin: 0;
  padding: var(--space-4);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  text-align: center;
}
</style>
