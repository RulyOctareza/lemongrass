<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import IconFrame from '../IconFrame.vue'
// An HTTP channel's type as a glyph: braces for api, a globe for web, a phone for app.
withDefaults(defineProps<{ type: 'api' | 'web' | 'app'; size?: number }>(), { size: 12 })
</script>

<template>
  <IconFrame :size="size" :box="16" :stroke-width="1.4">
    <template v-if="type === 'web'">
      <circle cx="8" cy="8" r="6.5" />
      <ellipse cx="8" cy="8" rx="2.8" ry="6.5" />
      <path d="M1.5 8H14.5" />
    </template>
    <template v-else-if="type === 'app'">
      <rect x="4" y="1.5" width="8" height="13" rx="1.6" />
      <path d="M7 12H9" />
    </template>
    <template v-else>
      <path
        d="M5.5 2C4 2 3.5 2.8 3.5 4V6.2C3.5 7.2 2.8 8 2 8C2.8 8 3.5 8.8 3.5 9.8V12C3.5 13.2 4 14 5.5 14"
      />
      <path
        d="M10.5 2C12 2 12.5 2.8 12.5 4V6.2C12.5 7.2 13.2 8 14 8C13.2 8 12.5 8.8 12.5 9.8V12C12.5 13.2 12 14 10.5 14"
      />
    </template>
  </IconFrame>
</template>
