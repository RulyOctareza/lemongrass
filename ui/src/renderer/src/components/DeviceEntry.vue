<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { openDevice, type ProjectRef } from '../workspace'

const props = defineProps<{
  project: ProjectRef
}>()
</script>

<template>
  <div class="device-panel">
    <button class="device-item" @click="openDevice(props.project)">
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
        <rect x="3.5" y="1.5" width="7" height="11" rx="1.4" />
        <path d="M6 10.5h2" />
      </svg>
      <span class="device-label">Device</span>
    </button>
  </div>
</template>

<style scoped>
.device-panel {
  flex-shrink: 0;
  padding: var(--space-2);
  border-top: 1px solid var(--color-border-default);
}

.device-item {
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
  transition: background var(--duration-fast) var(--ease-out);
}

.device-item:hover {
  background: var(--color-surface-3);
}
</style>
