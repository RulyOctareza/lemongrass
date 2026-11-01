<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import { contextLevel, seenText, tokenCount } from '../usage'
import type { ContextUsage } from '../../../preload/types'

const props = defineProps<{
  context: ContextUsage
}>()

const percent = computed(() =>
  props.context.window_tokens > 0
    ? (props.context.used_tokens / props.context.window_tokens) * 100
    : 0
)

const title = computed(() =>
  [
    `Context: ${props.context.used_tokens.toLocaleString()} of ${props.context.window_tokens.toLocaleString()} tokens (${Math.round(percent.value)}%).`,
    seenText(props.context.seen_at)
  ].join('\n')
)
</script>

<template>
  <span class="context-meter" :class="`context-meter-${contextLevel(percent)}`" :title="title">
    <span class="context-meter-track">
      <span class="context-meter-fill" :style="{ width: `${Math.min(100, percent)}%` }" />
    </span>
    <span class="context-meter-value"
      >{{ tokenCount(context.used_tokens) }} / {{ tokenCount(context.window_tokens) }}</span
    >
  </span>
</template>

<style scoped>
.context-meter {
  display: inline-flex;
  align-items: center;
  gap: var(--space-1);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-variant-numeric: tabular-nums;
  white-space: nowrap;
}

.context-meter-track {
  position: relative;
  width: 48px;
  height: 4px;
  overflow: hidden;
  border-radius: var(--radius-pill);
  background: var(--color-surface-3);
}

.context-meter-fill {
  position: absolute;
  inset: 0 auto 0 0;
  border-radius: var(--radius-pill);
  background: var(--color-fg-secondary);
}

.context-meter-warn .context-meter-fill {
  background: var(--color-orange);
}

.context-meter-warn .context-meter-value {
  color: var(--color-orange);
}

.context-meter-danger .context-meter-fill {
  background: var(--color-error);
}

.context-meter-danger .context-meter-value {
  color: var(--color-error);
  font-weight: 600;
}
</style>
