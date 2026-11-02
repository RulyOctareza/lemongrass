<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { openResterClient, type ProjectRef } from '../workspace'

const props = defineProps<{
  project: ProjectRef
}>()
</script>

<template>
  <div class="rester-client-panel">
    <button class="rester-client-item" @click="openResterClient(props.project)">
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
        <path d="M2 4.5h9.5L9.5 2.5" />
        <path d="M12 9.5H2.5l2 2" />
      </svg>
      <span class="rester-client-label">Rester Client</span>
    </button>
  </div>
</template>

<style scoped>
.rester-client-panel {
  flex-shrink: 0;
  padding: var(--space-2);
  border-top: 1px solid var(--color-border-default);
}

.rester-client-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--layer-raised);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.rester-client-item:hover {
  background: var(--layer-raised-hover);
}
</style>
