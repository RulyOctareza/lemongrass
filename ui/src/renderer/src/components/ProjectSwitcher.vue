<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed } from 'vue'
import AppSwitcher from './AppSwitcher.vue'
import type { Project } from '../../../preload/types'

const props = defineProps<{
  projects: Project[]
  activeProjectId: string | null
}>()

const emit = defineEmits<{
  select: [id: string]
  add: []
}>()

const items = computed(() =>
  props.projects.map((p) => ({ id: p.id, label: p.name, title: p.path }))
)
</script>

<template>
  <AppSwitcher
    :items="items"
    :active-id="activeProjectId"
    placeholder="No project"
    add-label="Add project"
    empty-text="No projects yet."
    @select="(id) => emit('select', id)"
    @add="emit('add')"
  />
</template>
