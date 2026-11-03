<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// An indeterminate progress line for work whose length is unknown, such as a running command. A
// bright segment with a soft glow slides along a muted track and pulses in brightness, so the
// state reads as alive at a glance. Pure CSS: nothing runs in script while it shows.
withDefaults(
  defineProps<{
    label: string
    tone?: 'jade' | 'amber'
  }>(),
  { tone: 'jade' }
)
</script>

<template>
  <div class="progress-line" :data-tone="tone" role="progressbar" :aria-label="label">
    <span class="segment" />
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
  left: -40%;
  width: 40%;
  border-radius: inherit;
  background: linear-gradient(
    90deg,
    transparent,
    var(--line-color) 30%,
    var(--line-color) 70%,
    transparent
  );
  box-shadow:
    0 0 6px var(--line-color),
    0 0 14px var(--line-muted);
  animation:
    progress-slide 1.6s cubic-bezier(0.45, 0, 0.55, 1) infinite,
    progress-pulse 1.2s ease-in-out infinite alternate;
}

@keyframes progress-slide {
  from {
    left: -40%;
  }
  to {
    left: 100%;
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
    left: 0;
    width: 100%;
    background: var(--line-color);
    animation: none;
  }
}
</style>
