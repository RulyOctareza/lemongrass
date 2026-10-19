<script setup lang="ts" generic="T extends string | number">
defineProps<{
  options: { value: T; label: string }[]
  modelValue: T
}>()

const emit = defineEmits<{
  'update:modelValue': [value: T]
}>()
</script>

<!-- Picks one of a few modes. Name the group with aria-label or aria-labelledby on the component. -->
<template>
  <div class="segmented" role="radiogroup">
    <button
      v-for="option in options"
      :key="option.value"
      type="button"
      role="radio"
      :aria-checked="option.value === modelValue"
      @click="emit('update:modelValue', option.value)"
    >
      {{ option.label }}
    </button>
  </div>
</template>

<style scoped>
.segmented {
  display: inline-flex;
  align-self: flex-start;
  flex-wrap: wrap;
  gap: 2px;
  padding: 3px;
  border-radius: var(--radius-md);
  background: var(--color-surface-3);
}

.segmented button {
  padding: var(--space-1) var(--space-3);
  border: none;
  border-radius: var(--radius-sm);
  background: transparent;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-default),
    color var(--duration-fast) var(--ease-default);
}

.segmented button:hover {
  color: var(--color-fg-primary);
}

.segmented button[aria-checked='true'] {
  background: var(--color-surface-1);
  color: var(--color-fg-primary);
  font-weight: var(--weight-semibold);
  box-shadow: var(--shadow-sm);
}

.segmented button:focus-visible {
  outline: 1px solid var(--color-amber);
  outline-offset: 1px;
}
</style>
