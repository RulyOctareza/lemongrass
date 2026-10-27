<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, onBeforeUnmount, ref, useAttrs } from 'vue'
import AppButton from './AppButton.vue'
import { CONFIRM_DELAY_MS, createClickConfirm, type ConfirmState } from '../clickConfirm'

// Every click-again confirm in the app goes through this button, so the wait, the progress bar
// and the armed red stay the same everywhere. The caller's class still sets size and layout.
defineOptions({ inheritAttrs: false })

const props = withDefaults(
  defineProps<{
    confirmLabel?: string
    // Renders through AppButton in that variant; without it the root is a bare button.
    variant?: 'primary' | 'ghost' | 'danger'
  }>(),
  { confirmLabel: 'Click again', variant: undefined }
)

const emit = defineEmits<{ confirm: [] }>()

const attrs = useAttrs()
const state = ref<ConfirmState>('idle')
const confirm = createClickConfirm((next) => (state.value = next))

const root = computed(() =>
  props.variant ? { is: AppButton, variant: props.variant } : { is: 'button' }
)
const armed = computed(() => state.value === 'armed')

function onClick(): void {
  if (confirm.press()) emit('confirm')
}

// Leaving resets only once armed, so a small hand movement over a small target does not cancel
// the wait without the user noticing.
function onLeave(): void {
  if (armed.value) confirm.reset()
}

function onEscape(event: KeyboardEvent): void {
  if (state.value === 'idle') return
  event.stopPropagation()
  confirm.reset()
}

onBeforeUnmount(() => confirm.reset())
</script>

<!-- type="button" comes first so a caller can override it, and so a confirm inside a form never
     submits it on the first click. -->
<template>
  <component
    :is="root.is"
    type="button"
    v-bind="{ ...attrs, ...(root.variant ? { variant: root.variant } : {}) }"
    class="app-confirm"
    :data-confirm="state"
    :title="armed ? confirmLabel : attrs.title"
    :aria-label="armed ? confirmLabel : attrs['aria-label']"
    :style="{ '--confirm-delay': `${CONFIRM_DELAY_MS}ms` }"
    @click="onClick"
    @blur="confirm.reset"
    @mouseleave="onLeave"
    @keydown.esc="onEscape"
  >
    <slot :state="state" :armed="armed" />
  </component>
</template>

<style scoped>
.app-confirm {
  position: relative;
  overflow: hidden;
}

.app-confirm[data-confirm='pending'] {
  cursor: progress;
}

.app-confirm[data-confirm='pending']::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  bottom: 0;
  height: 2px;
  background: var(--color-error);
  transform-origin: left;
  animation: confirm-fill var(--confirm-delay) linear forwards;
}

@keyframes confirm-fill {
  from {
    transform: scaleX(0);
  }
  to {
    transform: scaleX(1);
  }
}

/* !important because the armed look must win over any caller's hover, focus or colour rule. */
.app-confirm[data-confirm='armed'] {
  background: var(--color-error-muted) !important;
  border-color: var(--color-error) !important;
  color: var(--color-error) !important;
}

.app-confirm[data-confirm='armed'] :deep(*) {
  color: inherit !important;
}
</style>
