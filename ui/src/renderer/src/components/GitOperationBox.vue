<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { finishOperation, gitStateOf } from '../git'
import type { ProjectRef } from '../workspace'
import AppButton from './AppButton.vue'

const props = defineProps<{
  project: ProjectRef
}>()

const state = computed(() => gitStateOf(props.project.id))
const status = computed(() => state.value.status)
const error = ref<string | null>(null)
const label = computed(() => (status.value?.operation === 'merge' ? 'Merge' : 'Rebase'))
const hasConflicts = computed(() =>
  (status.value?.files ?? []).some((file) => file.group === 'conflicted')
)

async function finish(action: 'continue' | 'abort'): Promise<void> {
  error.value = await finishOperation(props.project, action)
}
</script>

<template>
  <div v-if="status?.operation" class="operation-box">
    <p>{{ label }} in progress. Resolve the conflicts, then continue or abort.</p>
    <p v-if="error" class="operation-error">{{ error }}</p>
    <div class="operation-buttons">
      <AppButton
        variant="primary"
        class="operation-action"
        :disabled="state.busy !== null || hasConflicts"
        @click="finish('continue')"
      >
        Continue
      </AppButton>
      <AppButton
        variant="danger"
        class="operation-action"
        :disabled="state.busy !== null"
        @click="finish('abort')"
      >
        Abort
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.operation-box {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  margin: 0 var(--space-2);
  padding: var(--space-2);
  background: var(--color-surface-2);
  border-radius: var(--radius-md);
  color: var(--color-warning);
  font-size: var(--text-xs);
}

.operation-error {
  max-height: 120px;
  overflow-y: auto;
  color: var(--color-error);
  white-space: pre-wrap;
  word-break: break-word;
}

.operation-buttons {
  display: flex;
  gap: var(--space-2);
}

.operation-buttons .operation-action {
  flex: 1;
  padding: var(--space-1) var(--space-2);
}
</style>
