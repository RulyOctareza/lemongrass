<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import IconFrame from './IconFrame.vue'
import { APP_ICONS, type AppGlyph, type AppIconName } from '../appIcons'

const props = withDefaults(defineProps<{ name: AppIconName; size?: number }>(), { size: 13 })

const glyph = computed((): AppGlyph => APP_ICONS[props.name])
</script>

<template>
  <IconFrame
    class="app-icon"
    :size="size"
    :box="glyph.box"
    :stroke-width="glyph.stroke"
    :fill="glyph.stroke === null ? 'currentColor' : 'none'"
  >
    <component
      :is="shape.tag"
      v-for="(shape, index) in glyph.shapes"
      :key="index"
      v-bind="shape.attrs"
    />
  </IconFrame>
</template>
