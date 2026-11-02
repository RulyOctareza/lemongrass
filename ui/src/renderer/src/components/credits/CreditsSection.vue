<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import AppButton from '../AppButton.vue'
import type { CreditEntry, CreditGroup, CreditsResult } from '../../../../preload/types'
import AppEmptyState from '../AppEmptyState.vue'

const GROUPS: { id: CreditGroup; label: string }[] = [
  { id: 'fonts', label: 'Fonts' },
  { id: 'app', label: 'App' },
  { id: 'tools', label: 'lgrass tools' },
  { id: 'electron', label: 'Electron and Chromium' }
]

const credits = ref<CreditsResult | null>(null)
const open = ref<CreditGroup[]>(GROUPS.map((g) => g.id))
const selected = ref<CreditEntry | null>(null)
const openError = ref('')

onMounted(async () => {
  credits.value = await window.api.credits.list()
})

const groups = computed(() =>
  GROUPS.map((g) => ({
    ...g,
    entries: (credits.value?.entries ?? []).filter((e) => e.group === g.id)
  })).filter((g) => g.entries.length > 0)
)

function toggle(id: CreditGroup): void {
  open.value = open.value.includes(id) ? open.value.filter((g) => g !== id) : [...open.value, id]
}

async function openChromium(): Promise<void> {
  openError.value = await window.api.credits.openChromiumLicenses()
}
</script>

<template>
  <div v-if="selected" class="credits">
    <div class="credits-detail-head">
      <AppButton type="button" @click="selected = null">Back</AppButton>
      <span class="settings-row-title">{{ selected.name }}</span>
      <span class="settings-row-meta">{{ selected.version }}</span>
      <span class="settings-badge">{{ selected.license }}</span>
    </div>
    <a
      v-if="selected.homepage"
      class="credits-link"
      :href="selected.homepage"
      target="_blank"
      rel="noreferrer"
      >{{ selected.homepage }}</a
    >
    <pre class="credits-text">{{ selected.text }}</pre>
  </div>

  <div v-else-if="credits" class="credits">
    <p class="settings-text">
      Lemongrass is built on the work of the open source projects below. Thank you to everyone who
      writes and maintains them.
    </p>
    <AppEmptyState
      v-if="credits.missing"
      title="This build has no credits list."
      detail="It is generated when the app is built with make dev or packaged."
    />
    <template v-else>
      <p class="settings-text">Select a component to read its full license.</p>
      <div v-for="group in groups" :key="group.id" class="settings-group">
        <button
          type="button"
          class="settings-group-head"
          :aria-expanded="open.includes(group.id)"
          @click="toggle(group.id)"
        >
          <span
            class="settings-group-chevron"
            :class="{ expanded: open.includes(group.id) }"
            aria-hidden="true"
            >▸</span
          >
          {{ group.label }}
          <span class="settings-group-count">{{ group.entries.length }}</span>
        </button>
        <div v-if="open.includes(group.id)" class="settings-group-body credits-body">
          <button
            v-for="entry in group.entries"
            :key="`${entry.name}@${entry.version}`"
            type="button"
            class="credits-row"
            @click="selected = entry"
          >
            <span class="settings-row-title">{{ entry.name }}</span>
            <span class="settings-row-meta">{{ entry.version }}</span>
            <span class="settings-badge">{{ entry.license }}</span>
          </button>
          <div v-if="group.id === 'electron'" class="credits-chromium">
            <span class="settings-text">
              Chromium and the libraries it uses are listed in a file installed with Lemongrass.
            </span>
            <AppButton type="button" @click="openChromium">Open Chromium licenses</AppButton>
          </div>
          <p v-if="group.id === 'electron' && openError" class="settings-error">{{ openError }}</p>
        </div>
      </div>
    </template>
  </div>
</template>

<style scoped>
.credits {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.credits-body {
  gap: var(--space-1);
  max-height: 320px;
}

.credits-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  width: 100%;
  padding: var(--space-1) var(--space-2);
  background: transparent;
  border: none;
  border-radius: var(--radius-md, 8px);
  font-family: var(--font-body);
  text-align: left;
  cursor: pointer;
}

.credits-row:hover {
  background: var(--color-surface-2);
}

.credits-row:focus-visible {
  outline: 1px solid var(--color-amber);
  outline-offset: -1px;
}

.credits-row .settings-row-title {
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.credits-chromium {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2);
}

.credits-chromium .settings-text {
  flex: 1;
}

.credits-detail-head {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.credits-link {
  color: var(--color-fg-accent);
  font-size: var(--text-xs);
  word-break: break-all;
}

.credits-text {
  margin: 0;
  padding: var(--space-3);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 8px);
  background: var(--color-surface-2);
  color: var(--color-fg-secondary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  line-height: var(--leading-relaxed);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
