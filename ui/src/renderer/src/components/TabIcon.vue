<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import type { IconShape } from '../tabKinds'

defineProps<{
  shapes: IconShape[]
}>()
</script>

<template>
  <svg
    class="tab-icon"
    width="13"
    height="13"
    viewBox="0 0 14 14"
    fill="none"
    stroke="currentColor"
    stroke-width="1.2"
    stroke-linecap="round"
    stroke-linejoin="round"
  >
    <component :is="shape.tag" v-for="(shape, index) in shapes" :key="index" v-bind="shape.attrs" />
  </svg>
</template>

<style scoped>
.tab-icon {
  flex-shrink: 0;
}
</style>
