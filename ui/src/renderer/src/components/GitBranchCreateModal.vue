<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, nextTick, onMounted, ref, watch } from 'vue'
import { createBranch } from '../git'
import type { ProjectRef } from '../workspace'
import type { GitBranches } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppModal from './AppModal.vue'
import AppSwitcher from './AppSwitcher.vue'

const props = defineProps<{
  project: ProjectRef
  hasChanges: boolean
}>()

const emit = defineEmits<{
  close: []
}>()

const DETACHED = 'HEAD'

const branches = ref<GitBranches | null>(null)
const name = ref('')
const base = ref<string | null>(null)
const nameValid = ref(false)
const error = ref<string | null>(null)
const busy = ref(false)
const nameRef = ref<HTMLInputElement | null>(null)

const trimmed = computed(() => name.value.trim())
const exists = computed(
  () => !!branches.value?.local.some((branch) => branch.name === trimmed.value)
)

// A detached HEAD has no branch to start from, so it is offered under its own name.
const baseItems = computed(() => {
  const list = branches.value
  if (!list) return []
  const items = [...list.local.map((branch) => branch.name), ...list.remote].map((ref) => ({
    id: ref,
    label: ref
  }))
  return list.current ? items : [{ id: DETACHED, label: 'Detached HEAD' }, ...items]
})

const nameProblem = computed((): string | null => {
  if (!trimmed.value) return null
  if (exists.value) return 'A local branch already has this name.'
  if (!nameValid.value) return 'This is not a valid branch name.'
  return null
})

const canCreate = computed(
  () => !!trimmed.value && !nameProblem.value && !!base.value && !busy.value
)

onMounted(async () => {
  await nextTick()
  nameRef.value?.focus()
  branches.value = await window.api.git.branches(props.project.path)
  base.value = branches.value.current ?? DETACHED
})

// Main runs git check-ref-format, so only the answer for the latest text is kept.
let nameCheck = 0
watch(trimmed, async (value) => {
  const check = ++nameCheck
  nameValid.value = false
  if (!value) return
  const valid = await window.api.git.branchNameValid(props.project.path, value)
  if (check === nameCheck) nameValid.value = valid
})

async function create(): Promise<void> {
  if (!canCreate.value || !base.value) return
  error.value = null
  busy.value = true
  error.value = await createBranch(props.project, trimmed.value, base.value)
  busy.value = false
  if (!error.value) emit('close')
}
</script>

<template>
  <AppModal title="New branch" @close="emit('close')">
    <label class="create-field">
      <span class="create-label">Name</span>
      <input
        ref="nameRef"
        v-model="name"
        class="create-input"
        placeholder="feature/new-thing"
        aria-label="Branch name"
        :disabled="busy"
        @keydown.enter="create"
      />
    </label>
    <p v-if="nameProblem" class="create-error">{{ nameProblem }}</p>

    <div class="create-field">
      <span class="create-label">From</span>
      <AppSwitcher
        framed
        :items="baseItems"
        :active-id="base"
        :placeholder="branches ? 'Pick a branch' : 'Loading...'"
        empty-text="No branches."
        @select="base = $event"
      />
    </div>

    <p v-if="hasChanges" class="create-note">
      Your uncommitted changes come along to the new branch.
    </p>
    <p v-if="error" class="create-error">{{ error }}</p>

    <template #footer>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton variant="primary" :disabled="!canCreate" @click="create">Create</AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.create-field {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.create-label {
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.create-input {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--input-bg, var(--color-surface-0));
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
}

.create-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.create-note {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.create-error {
  color: var(--color-error);
  font-size: var(--text-sm);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
