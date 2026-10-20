<script setup lang="ts">
import { onMounted, ref } from 'vue'
import type { ProjectRef } from '../workspace'
import type { GitCommit } from '../../../preload/types'
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

const message = ref('')
const loaded = ref(false)
const error = ref<string | null>(null)
const busy = ref(false)

onMounted(async () => {
  const info = await window.api.git.commitInfo(props.project.path, props.commit.hash)
  message.value = info?.message ?? props.commit.subject
  loaded.value = true
})

async function save(): Promise<void> {
  busy.value = true
  error.value = null
  const result = await window.api.git.amendMessage(props.project.path, message.value)
  busy.value = false
  if (result.ok) emit('done')
  else error.value = result.error
}
</script>

<template>
  <AppModal title="Amend commit message" @close="emit('close')">
    <textarea
      v-model="message"
      class="amend-input"
      rows="6"
      aria-label="Commit message"
      :disabled="!loaded || busy"
    />
    <p v-if="!commit.unpushed" class="amend-warning">
      This commit is already on a remote. Pushing the amended commit is rejected, and this app never
      forces a push.
    </p>
    <p v-if="error" class="amend-error">{{ error }}</p>
    <template #footer>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton
        variant="primary"
        :disabled="!loaded || busy || message.trim().length === 0"
        @click="save"
      >
        Amend
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.amend-input {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  resize: vertical;
}

.amend-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.amend-warning {
  margin-top: var(--space-3);
  color: var(--color-warning);
  font-size: var(--text-sm);
}

.amend-error {
  margin-top: var(--space-3);
  color: var(--color-error);
  font-size: var(--text-sm);
  white-space: pre-wrap;
}
</style>
