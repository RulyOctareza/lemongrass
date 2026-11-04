<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The one empty, loading and no-match state. fill is for a view or pane whose whole content is
// missing and centers itself in the space it is given, so its parent must let it grow (a flex
// column, or a box with a height). inline stands in a list's place as a dashed ghost card.
import AppIcon from './AppIcon.vue'
import type { IconShape } from '../tabKinds'

withDefaults(
  defineProps<{
    layout?: 'fill' | 'inline'
    size?: 'sm' | 'md'
    title?: string
    detail?: string
    loading?: boolean
    // Stroke shapes in a 14 by 14 box, the same shape data tab icons use. Shown on fill only.
    icon?: IconShape[]
  }>(),
  {
    layout: 'inline',
    size: 'md',
    title: '',
    detail: '',
    loading: false,
    icon: undefined
  }
)
</script>

<template>
  <div
    class="empty-state"
    :class="[`empty-state--${layout}`, `empty-state--${size}`]"
    :role="loading ? 'status' : undefined"
  >
    <svg
      v-if="layout === 'fill' && icon && !loading"
      class="empty-icon"
      width="28"
      height="28"
      viewBox="0 0 14 14"
      fill="none"
      stroke="currentColor"
      stroke-width="0.9"
      stroke-linecap="round"
      stroke-linejoin="round"
      aria-hidden="true"
    >
      <component :is="shape.tag" v-for="(shape, i) in icon" :key="i" v-bind="shape.attrs" />
    </svg>
    <p class="empty-title">
      <AppIcon v-if="loading" class="empty-spinner" name="spinner" :size="12" />
      <slot>{{ title }}</slot>
    </p>
    <p v-if="detail || $slots.detail" class="empty-detail">
      <slot name="detail">{{ detail }}</slot>
    </p>
    <div v-if="$slots.action" class="empty-action">
      <slot name="action" />
    </div>
  </div>
</template>

<style scoped>
.empty-state {
  display: flex;
  flex-direction: column;
  align-items: center;
  justify-content: center;
  gap: var(--space-1);
  box-sizing: border-box;
  text-align: center;
}

.empty-state--fill {
  flex: 1;
  align-self: stretch;
  width: 100%;
  min-height: 160px;
  padding: var(--space-8) var(--space-6);
}

.empty-state--inline {
  padding: var(--space-6) var(--space-4);
  border: 1px dashed var(--color-border-default);
  border-radius: var(--radius-lg);
}

.empty-state--inline.empty-state--sm {
  padding: var(--space-3) var(--space-2);
  border-radius: var(--radius-md);
}

.empty-icon {
  margin-bottom: var(--space-2);
  color: var(--color-fg-muted);
}

.empty-title {
  display: inline-flex;
  align-items: center;
  gap: var(--space-2);
  margin: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  font-weight: var(--weight-medium);
  line-height: 1.4;
}

.empty-detail {
  max-width: 420px;
  margin: 0;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
  line-height: 1.5;
}

.empty-state--sm .empty-title {
  font-size: var(--text-xs);
}

.empty-action {
  margin-top: var(--space-3);
}

.empty-spinner {
  flex-shrink: 0;
  color: var(--color-fg-muted);
  animation: spin 0.9s linear infinite;
}
</style>
