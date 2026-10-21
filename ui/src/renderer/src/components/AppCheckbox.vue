<script setup lang="ts">
// Indeterminate is the partly-checked state of a select-all box and wins over checked.
defineProps<{
  checked: boolean
  indeterminate?: boolean
}>()
</script>

<template>
  <button
    type="button"
    role="checkbox"
    class="app-check"
    :aria-checked="indeterminate ? 'mixed' : checked"
  >
    <span class="box" aria-hidden="true">
      <svg
        width="10"
        height="10"
        viewBox="0 0 24 24"
        fill="none"
        stroke="currentColor"
        stroke-width="3.5"
        stroke-linecap="round"
        stroke-linejoin="round"
      >
        <line v-if="indeterminate" x1="6" y1="12" x2="18" y2="12" />
        <polyline v-else points="20 6 9 17 4 12" />
      </svg>
    </span>
    <span v-if="$slots.default" class="label"><slot /></span>
  </button>
</template>

<style scoped>
.app-check {
  display: flex;
  align-items: flex-start;
  gap: var(--space-2);
  padding: var(--space-1) 0;
  background: none;
  border: none;
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
  user-select: none;
}

.box {
  width: 16px;
  height: 16px;
  flex-shrink: 0;
  margin-top: 1px;
  display: flex;
  align-items: center;
  justify-content: center;
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-strong);
  border-radius: var(--radius-sm);
  color: var(--color-black);
  transition:
    background var(--duration-fast) var(--ease-default),
    border-color var(--duration-fast) var(--ease-default);
}

.box svg {
  opacity: 0;
}

.app-check[aria-checked='true'] .box,
.app-check[aria-checked='mixed'] .box {
  background: var(--color-amber);
  border-color: var(--color-amber);
}

.app-check[aria-checked='true'] .box svg,
.app-check[aria-checked='mixed'] .box svg {
  opacity: 1;
}

.app-check:focus-visible {
  outline: 1px solid var(--color-amber);
  outline-offset: 2px;
  border-radius: var(--radius-sm);
}

.label {
  min-width: 0;
  word-break: break-all;
}
</style>
