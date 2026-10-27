<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { ProjectRef } from '../workspace'
import type { GitCommit, GitResetPreview } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppModal from './AppModal.vue'

const props = defineProps<{
  project: ProjectRef
  commit: GitCommit
}>()

const emit = defineEmits<{
  close: []
  done: []
}>()

const preview = ref<GitResetPreview | null>(null)
const error = ref<string | null>(null)
const busy = ref(false)

onMounted(async () => {
  preview.value = await window.api.git.resetPreview(props.project.path, props.commit.hash)
})

async function reset(): Promise<void> {
  busy.value = true
  const result = await window.api.git.resetSoft(props.project.path, props.commit.hash)
  busy.value = false
  if (result.ok) emit('done')
  else error.value = result.error
}
</script>

<template>
  <AppModal title="Reset soft to this commit" @close="emit('close')">
    <p class="reset-target">{{ commit.hash.slice(0, 7) }} {{ commit.subject }}</p>
    <p v-if="!preview" class="reset-note">Checking...</p>
    <p v-else-if="!preview.ok" class="reset-error">{{ preview.error }}</p>
    <template v-else>
      <p class="reset-note">
        The branch moves back {{ preview.count }} {{ preview.count === 1 ? 'commit' : 'commits' }}.
        Their changes stay staged, and nothing in the working tree is touched.
      </p>
      <p v-if="preview.pushed > 0" class="reset-warning">
        {{ preview.pushed }} of them {{ preview.pushed === 1 ? 'is' : 'are' }} already on a remote.
        Pushing the branch afterwards is rejected, and this app never forces a push.
      </p>
    </template>
    <p v-if="error" class="reset-error">{{ error }}</p>
    <template #footer>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton variant="danger" :disabled="!preview?.ok || busy" @click="reset">
        Reset soft
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.reset-target {
  margin-bottom: var(--space-3);
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
  word-break: break-word;
}

.reset-note {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.reset-warning {
  margin-top: var(--space-3);
  color: var(--color-warning);
  font-size: var(--text-sm);
}

.reset-error {
  margin-top: var(--space-3);
  color: var(--color-error);
  font-size: var(--text-sm);
  white-space: pre-wrap;
}
</style>
