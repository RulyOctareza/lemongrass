<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The svg every icon component draws into, so size, stroke and accessibility are set one way. size
// is the rendered height in px; the width follows the box's aspect, and the stroke scales with it.
// A null strokeWidth draws a filled glyph.
const props = withDefaults(
  defineProps<{
    size: number
    box?: number | readonly [number, number]
    strokeWidth?: number | null
    fill?: string
  }>(),
  { box: 14, strokeWidth: 1.2, fill: 'none' }
)

function dims(): [number, number] {
  return typeof props.box === 'number' ? [props.box, props.box] : [props.box[0], props.box[1]]
}
</script>

<template>
  <svg
    class="icon-frame"
    :width="(size * dims()[0]) / dims()[1]"
    :height="size"
    :viewBox="`0 0 ${dims()[0]} ${dims()[1]}`"
    :fill="fill"
    :stroke="strokeWidth === null ? undefined : 'currentColor'"
    :stroke-width="strokeWidth ?? undefined"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <slot />
  </svg>
</template>

<style scoped>
.icon-frame {
  flex-shrink: 0;
}
</style>
