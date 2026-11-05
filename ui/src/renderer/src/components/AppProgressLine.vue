<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
// A progress line. Without a value it is indeterminate, for work whose length is unknown such as a
// running command: a bright line with a soft glow grows from the left edge until it fills the muted
// track, then shrinks toward the right edge. With a value from 0 to 1 it fills from the left to that
// fraction instead. Both pulse in brightness, so the state reads as alive at a glance. Pure CSS:
// nothing runs in script while it shows.
const props = withDefaults(
  defineProps<{
    label: string
    tone?: 'jade' | 'amber'
    value?: number | null
  }>(),
  { tone: 'jade', value: null }
)

const fill = computed(() =>
  props.value === null ? null : `${(1 - Math.min(Math.max(props.value, 0), 1)) * 100}%`
)
</script>

<template>
  <div
    class="progress-line"
    :data-tone="tone"
    role="progressbar"
    :aria-label="label"
    :aria-valuemin="fill === null ? undefined : 0"
    :aria-valuemax="fill === null ? undefined : 100"
    :aria-valuenow="fill === null ? undefined : Math.round((value ?? 0) * 100)"
  >
    <span v-if="fill === null" class="segment" />
    <span v-else class="segment determinate" :style="{ right: fill }" />
  </div>
</template>

<style scoped>
.progress-line {
  --line-color: var(--color-jade);
  --line-muted: var(--color-jade-muted);
  position: relative;
  height: 3px;
  border-radius: var(--radius-pill);
  background: var(--line-muted);
  /* Clipped to the track along its length, with room above and below for the glow. */
  overflow: clip;
  overflow-clip-margin: 8px;
}

.progress-line[data-tone='amber'] {
  --line-color: var(--color-amber);
  --line-muted: var(--color-amber-muted);
}

.segment {
  position: absolute;
  top: 0;
  bottom: 0;
  left: 0;
  right: 100%;
  border-radius: inherit;
  background: var(--line-color);
  box-shadow:
    0 0 6px var(--line-color),
    0 0 14px var(--line-muted);
  animation:
    progress-sweep 2s infinite,
    progress-pulse 1.2s ease-in-out infinite alternate;
}

/* Anchored at the left edge and eased toward each new fraction; only the brightness pulse runs. */
.segment.determinate {
  animation: progress-pulse 1.2s ease-in-out infinite alternate;
  transition: right var(--duration-base) var(--ease-out);
}

/* Grow from the left edge, then let go of it toward the right; each half eases on its own, so the
   line slows into the full track and out of it instead of turning sharply. */
@keyframes progress-sweep {
  0% {
    left: 0;
    right: 100%;
    animation-timing-function: cubic-bezier(0.45, 0, 0.55, 1);
  }
  50% {
    left: 0;
    right: 0;
    animation-timing-function: cubic-bezier(0.45, 0, 0.55, 1);
  }
  100% {
    left: 100%;
    right: 0;
  }
}

@keyframes progress-pulse {
  from {
    filter: brightness(0.9);
  }
  to {
    filter: brightness(1.35);
  }
}

/* Still, but plainly on: the whole line in the tone color with its glow. */
@media (prefers-reduced-motion: reduce) {
  .segment {
    right: 0;
    animation: none;
  }

  .segment.determinate {
    transition: none;
  }
}
</style>
