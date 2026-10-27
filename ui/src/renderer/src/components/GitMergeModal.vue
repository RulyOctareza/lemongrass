<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { gitStateOf, mergeInto } from '../git'
import type { ProjectRef } from '../workspace'
import AppButton from './AppButton.vue'
import AppModal from './AppModal.vue'

const props = defineProps<{
  project: ProjectRef
  source: string
}>()

const emit = defineEmits<{
  close: []
}>()

const state = computed(() => gitStateOf(props.project.id))
const target = computed(() => state.value.status?.branch ?? 'HEAD')
const count = ref<number | null>(null)
const error = ref<string | null>(null)

onMounted(async () => {
  count.value = await window.api.git.mergeCount(props.project.path, props.source)
})

const summary = computed(() => {
  if (count.value === null) return 'Counting incoming commits...'
  if (count.value === 0) return `${target.value} already has every commit of ${props.source}.`
  const commits = count.value === 1 ? '1 commit' : `${count.value} commits`
  return `${commits} from ${props.source} will come into ${target.value}.`
})

async function merge(): Promise<void> {
  error.value = await mergeInto(props.project, props.source)
  // A merge stopped on conflicts is now the panel's Continue and Abort box to finish.
  if (!error.value || state.value.status?.operation) emit('close')
}
</script>

<template>
  <AppModal :title="`Merge ${source} into ${target}`" @close="emit('close')">
    <p class="merge-note">{{ summary }}</p>
    <p v-if="error" class="merge-error">{{ error }}</p>

    <template #footer>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton variant="primary" :disabled="!count || state.busy !== null" @click="merge">
        Merge
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.merge-note {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.merge-error {
  color: var(--color-error);
  font-size: var(--text-sm);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
