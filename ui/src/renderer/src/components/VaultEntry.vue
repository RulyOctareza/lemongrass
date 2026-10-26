<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { vaultSession } from '../vaultSession'

const emit = defineEmits<{
  open: []
}>()
</script>

<template>
  <div class="vault-panel">
    <button class="vault-item" :class="{ open: vaultSession.unlocked }" @click="emit('open')">
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
        <rect x="2.5" y="6.5" width="9" height="6" rx="1.2" />
        <path d="M4.5 6.5V4.5a2.5 2.5 0 0 1 5 0v2" />
        <circle cx="7" cy="9.5" r="0.8" />
      </svg>
      <span class="vault-label">Vault</span>
      <span class="vault-state">{{ vaultSession.unlocked ? 'Unlocked' : 'Locked' }}</span>
    </button>
  </div>
</template>

<style scoped>
.vault-panel {
  flex-shrink: 0;
  padding: var(--space-2);
  border-top: 1px solid var(--color-border-default);
}

.vault-item {
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

.vault-item:hover {
  background: var(--color-surface-3);
}

.vault-item.open {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.vault-label {
  flex-shrink: 0;
}

.vault-state {
  flex: 1;
  min-width: 0;
  color: var(--color-fg-secondary);
  font-weight: var(--weight-regular);
  text-align: left;
}
</style>
