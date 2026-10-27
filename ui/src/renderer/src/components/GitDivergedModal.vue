<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { gitStateOf, rebaseOntoUpstream, type GitDiverged } from '../git'
import type { ProjectRef } from '../workspace'
import AppButton from './AppButton.vue'
import AppModal from './AppModal.vue'
import GitCounts from './GitCounts.vue'

const props = defineProps<{
  project: ProjectRef
  diverged: GitDiverged
}>()

const emit = defineEmits<{
  close: []
}>()

const state = computed(() => gitStateOf(props.project.id))
const isCurrent = computed(() => state.value.status?.branch === props.diverged.branch)
const error = ref<string | null>(null)

function commits(count: number): string {
  return count === 1 ? '1 commit' : `${count} commits`
}

async function rebase(): Promise<void> {
  error.value = await rebaseOntoUpstream(props.project, props.diverged.branch)
  // A rebase stopped on conflicts is now the panel's Continue and Abort box to finish.
  if (!error.value || state.value.status?.operation) emit('close')
}
</script>

<template>
  <AppModal :title="`${diverged.branch} has diverged`" @close="emit('close')">
    <div class="diverged-sides">
      <span class="diverged-name">{{ diverged.branch }}</span>
      <GitCounts :ahead="diverged.ahead" :behind="diverged.behind" />
      <span class="diverged-name">{{ diverged.upstream }}</span>
    </div>
    <p class="diverged-note">
      {{ diverged.branch }} has {{ commits(diverged.ahead) }} that {{ diverged.upstream }} does not,
      and {{ diverged.upstream }} has {{ commits(diverged.behind) }} that {{ diverged.branch }} does
      not. A rebase replays the {{ diverged.branch }} commits on top of {{ diverged.upstream }},
      after which it can be pushed.
    </p>
    <p v-if="!isCurrent" class="diverged-note">
      The rebase checks out {{ diverged.branch }} first.
    </p>
    <p class="diverged-warning">
      A rebase rewrites those commits. That is safe while they are not on {{ diverged.upstream }}
      yet, but one already pushed to another branch could not be pushed there again without a force
      push, which this app never does.
    </p>
    <p v-if="error" class="diverged-error">{{ error }}</p>

    <template #footer>
      <AppButton @click="emit('close')">Cancel</AppButton>
      <AppButton variant="danger" :disabled="state.busy !== null" @click="rebase">
        Rebase onto {{ diverged.upstream }}
      </AppButton>
    </template>
  </AppModal>
</template>

<style scoped>
.diverged-sides {
  display: flex;
  align-items: center;
  gap: var(--space-2);
  font-size: var(--text-sm);
}

.diverged-name {
  color: var(--color-fg-primary);
  font-weight: var(--weight-semibold);
}

.diverged-note {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.diverged-warning {
  color: var(--color-warning);
  font-size: var(--text-sm);
}

.diverged-error {
  color: var(--color-error);
  font-size: var(--text-sm);
  white-space: pre-wrap;
  word-break: break-word;
}
</style>
