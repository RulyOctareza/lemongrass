<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The hint card shown while its anchor, an info icon or a capsule, is hovered or focused. It renders
// in body, fixed under the anchor or above it when there is no room, so a modal never crops it.
// The parent sets anchor on mouseenter or focus and clears it on mouseleave or blur.
import { nextTick, onBeforeUnmount, ref, watch } from 'vue'
import { placeFloat, placementStyle } from '../floatPlace'

const props = defineProps<{
  anchor: HTMLElement | null
  id: string
}>()

const cardRef = ref<HTMLElement | null>(null)
const style = ref<Record<string, string>>({})
const placed = ref(false)

function place(): void {
  const anchor = props.anchor
  const card = cardRef.value
  if (!anchor || !card) return
  const p = placeFloat(anchor.getBoundingClientRect(), card.offsetWidth, card.offsetHeight, 4)
  style.value = placementStyle(p)
  placed.value = true
}

function listen(on: boolean): void {
  const method = on ? 'addEventListener' : 'removeEventListener'
  window[method]('resize', place)
  document[method]('scroll', place, true)
}

watch(
  () => props.anchor,
  async (anchor, previous) => {
    if (!anchor) {
      placed.value = false
      listen(false)
      return
    }
    if (!previous) listen(true)
    placed.value = false
    style.value = {}
    await nextTick()
    place()
  }
)

onBeforeUnmount(() => listen(false))
</script>

<template>
  <Teleport to="body">
    <span
      v-if="anchor"
      :id="id"
      ref="cardRef"
      class="app-info-popover"
      :class="{ placed }"
      role="tooltip"
      :style="style"
    >
      <slot />
    </span>
  </Teleport>
</template>

<style scoped>
/* The vault login's info-popover look. Above the app modal, below the app's closing overlay. */
.app-info-popover {
  position: fixed;
  top: 0;
  left: 0;
  z-index: 950;
  width: 260px;
  padding: var(--space-3);
  background: var(--layer-float);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  box-shadow: var(--shadow-md);
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: 1.5;
  overflow-wrap: anywhere;
  pointer-events: none;
  visibility: hidden;
}

.app-info-popover.placed {
  visibility: visible;
}
</style>
