<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// Renders a sentence built from NameParts. Every part is interpolated as text, never as HTML, since
// a marked part is often a tab title a model wrote.
import { computed } from 'vue'
import { capitalizeParts, type NamePart } from '../tabNames'

const props = defineProps<{
  parts: NamePart[]
  capitalize?: boolean
}>()

const shown = computed(() => (props.capitalize ? capitalizeParts(props.parts) : props.parts))
</script>

<template>
  <span class="marked-text"
    ><template v-for="(part, i) in shown" :key="i"
      ><strong v-if="part.mark" class="mark">{{ part.text }}</strong
      ><template v-else>{{ part.text }}</template></template
    ></span
  >
</template>

<style scoped>
.mark {
  color: var(--color-fg-accent);
  font-weight: var(--weight-semibold);
}
</style>
