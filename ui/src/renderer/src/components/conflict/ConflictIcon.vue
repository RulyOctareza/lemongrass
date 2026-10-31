<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { CONFLICT_ICONS, type ConflictIconName } from '../../conflictIcons'

defineProps<{
  name: ConflictIconName
}>()
</script>

<template>
  <svg
    class="conflict-icon"
    width="13"
    height="13"
    viewBox="0 0 14 14"
    fill="none"
    stroke="currentColor"
    stroke-width="1.3"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path v-for="d in CONFLICT_ICONS[name]" :key="d" :d="d" />
  </svg>
</template>

<style scoped>
.conflict-icon {
  flex-shrink: 0;
}
</style>
