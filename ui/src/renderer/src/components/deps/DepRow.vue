<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import AppButton from '../AppButton.vue'
import {
  actionLabel,
  buttonWeight,
  featuresText,
  originText,
  routeBlocked,
  routeNote,
  statusText,
  tone
} from '../../depsView'
import type { DepEntry, DepsState } from '../../../../preload/types'

const props = defineProps<{
  dep: DepEntry
  state: DepsState
}>()

const installing = computed(() =>
  props.state.installing?.name === props.dep.name ? props.state.installing : null
)
const outcome = computed(() => props.state.outcomes[props.dep.name] ?? null)
const weight = computed(() => buttonWeight(props.dep))
const blocked = computed(() => (props.dep.offer ? routeBlocked(props.dep.offer) : null))

// One install at a time, since the package manager holds a single lock, and none while a check runs.
const disabled = computed(
  () =>
    !!props.state.unsupported ||
    !!blocked.value ||
    !!props.state.installing ||
    props.state.phase === 'checking'
)

const note = computed(() => (props.dep.offer && !blocked.value ? routeNote(props.dep.offer) : null))

const meta = computed(() => [props.dep.path, originText(props.dep)].filter(Boolean).join(', '))

function install(): void {
  void window.api.deps.install(props.dep.name)
}
</script>

<template>
  <li class="dep-row">
    <div class="dep-head">
      <span class="settings-row-title">{{ dep.name }}</span>
      <span class="dep-status" :class="tone(dep)">{{ statusText(dep) }}</span>
      <span v-if="!dep.required" class="settings-badge">Optional</span>
      <span class="dep-spacer" />
      <AppButton
        v-if="weight && dep.offer"
        type="button"
        :variant="weight"
        :disabled="disabled"
        :title="blocked ?? undefined"
        @click="install"
      >
        {{ installing ? 'Working' : `${actionLabel(dep.offer)} ${dep.offer.package}` }}
      </AppButton>
    </div>
    <p v-if="meta" class="settings-row-meta dep-meta" :title="meta">{{ meta }}</p>
    <p v-if="dep.features.length" class="dep-text">{{ featuresText(dep.features) }}</p>
    <p v-if="dep.message" class="dep-text dep-message">{{ dep.message }}</p>
    <p v-if="blocked && weight" class="dep-text">{{ blocked }}</p>
    <p v-if="note && weight && !installing" class="dep-text">{{ note }}</p>
    <div v-if="installing" class="dep-output">
      <p class="dep-text">
        {{ installing.action === 'upgrade' ? 'Updating' : 'Installing' }} {{ installing.package }}.
      </p>
      <pre v-if="installing.lines.length" class="dep-lines">{{ installing.lines.join('\n') }}</pre>
    </div>
    <p v-else-if="outcome" class="dep-text" :class="outcome.ok ? 'dep-ok' : 'settings-error'">
      {{ outcome.text }}
    </p>
  </li>
</template>

<style scoped>
.dep-row {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 8px);
}

.dep-head {
  display: flex;
  align-items: center;
  gap: var(--space-3);
}

.dep-spacer {
  flex: 1;
}

.dep-status {
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.dep-status.ok {
  color: var(--color-success);
}

.dep-status.warn {
  color: var(--color-warning);
}

.dep-status.muted {
  color: var(--color-fg-muted);
}

.dep-meta {
  flex: none;
  margin: 0;
}

.dep-text {
  margin: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  line-height: var(--leading-relaxed);
}

.dep-message {
  color: var(--color-fg-primary);
}

.dep-ok {
  color: var(--color-success);
}

.dep-output {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.dep-lines {
  margin: 0;
  padding: var(--space-2);
  overflow-x: auto;
  background: var(--color-surface-0);
  border-radius: var(--radius-sm, 4px);
  color: var(--color-fg-muted);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  white-space: pre;
}
</style>
