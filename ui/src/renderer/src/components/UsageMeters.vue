<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import { resetText, seenText, usageLevel } from '../usage'
import type { UsageWindow } from '../../../preload/types'

const props = defineProps<{
  windows: UsageWindow[] | undefined
  seenAt: number | undefined
  // Compact leaves the age out of the row; it stays in the hover title.
  compact?: boolean
}>()

const title = computed(() =>
  props.windows && props.seenAt
    ? [...props.windows.map((w) => resetText(w)), seenText(props.seenAt)].join('\n')
    : 'No usage reported yet. It appears after this account answers a prompt.'
)
</script>

<template>
  <span class="usage-meters" :title="title">
    <template v-if="windows && windows.length > 0">
      <span
        v-for="w in windows"
        :key="w.label"
        class="usage-meter"
        :class="`usage-meter-${usageLevel([w])}`"
      >
        <span class="usage-meter-label">{{ w.label }}</span>
        <span class="usage-meter-track">
          <span class="usage-meter-fill" :style="{ width: `${Math.min(100, w.used_percent)}%` }" />
        </span>
        <span class="usage-meter-value">{{
          w.reset ? 'reset' : `${Math.round(w.used_percent)}%`
        }}</span>
      </span>
      <span v-if="seenAt && !compact" class="usage-meter-seen">{{ seenText(seenAt) }}</span>
    </template>
    <span v-else class="usage-meter-none">No usage yet</span>
  </span>
</template>

<style scoped>
.usage-meters {
  display: inline-flex;
  align-items: center;
  gap: var(--space-3);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.usage-meter {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
}

.usage-meter-track {
  position: relative;
  width: 48px;
  height: 4px;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--color-surface-3);
}

.usage-meter-fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: var(--radius-pill);
  background: var(--color-fg-secondary);
}

.usage-meter-value {
  min-width: 3ch;
  text-align: right;
}

.usage-meter-warn .usage-meter-fill {
  background: var(--color-warning);
}

.usage-meter-warn .usage-meter-value {
  color: var(--color-warning);
}

.usage-meter-danger .usage-meter-fill {
  background: var(--color-error);
}

.usage-meter-danger .usage-meter-value {
  color: var(--color-error);
  font-weight: 600;
}

.usage-meter-seen,
.usage-meter-none {
  color: var(--color-fg-muted);
}
</style>
