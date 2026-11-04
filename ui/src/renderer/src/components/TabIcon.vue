<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import IconFrame from './IconFrame.vue'
import type { IconShape } from '../tabKinds'

withDefaults(defineProps<{ shapes: IconShape[]; size?: number }>(), { size: 13 })
</script>

<template>
  <IconFrame class="tab-icon" :size="size">
    <component :is="shape.tag" v-for="(shape, index) in shapes" :key="index" v-bind="shape.attrs" />
  </IconFrame>
</template>
