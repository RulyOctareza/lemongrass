<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import { gitStateOf, runStash } from '../git'
import { formatAge } from '../gitTime'
import type { ProjectRef } from '../workspace'
import type { GitStash } from '../../../preload/types'
import AppButton from './AppButton.vue'
import AppCheckbox from './AppCheckbox.vue'
import AppModal from './AppModal.vue'
import GitIcon from './GitIcon.vue'
import RevisionView from './RevisionView.vue'

const props = defineProps<{
  project: ProjectRef
  hasChanges: boolean
}>()

const state = computed(() => gitStateOf(props.project.id))
const stashes = computed(() => state.value.stashes)
const folded = ref(true)
const saving = ref(false)
const message = ref('')
const includeUntracked = ref(true)
const error = ref<string | null>(null)
const confirmingDrop = ref<string | null>(null)
const viewing = ref<GitStash | null>(null)

async function perform(
  action: 'apply' | 'pop' | 'drop' | { push: string; includeUntracked: boolean },
  ref = ''
): Promise<boolean> {
  error.value = null
  const failure = await runStash(props.project, action, ref)
  error.value = failure
  return failure === null
}

async function save(): Promise<void> {
  const text = message.value
  if (await perform({ push: text, includeUntracked: includeUntracked.value })) {
    message.value = ''
    saving.value = false
  }
}

async function drop(stash: GitStash): Promise<void> {
  if (confirmingDrop.value !== stash.ref) {
    confirmingDrop.value = stash.ref
    return
  }
  confirmingDrop.value = null
  await perform('drop', stash.ref)
}
</script>

<template>
  <div class="stash-section">
    <div class="stash-head">
      <button class="stash-toggle" :aria-expanded="!folded" @click="folded = !folded">
        <svg
          class="stash-chevron"
          :class="{ open: !folded }"
          width="9"
          height="9"
          viewBox="0 0 10 10"
          fill="none"
          stroke="currentColor"
          stroke-width="1.2"
          stroke-linecap="round"
          stroke-linejoin="round"
        >
          <path d="M3 2l3 3-3 3" />
        </svg>
        <span class="git-heading">Stashes</span>
        <span v-if="stashes.length > 0" class="stash-count">{{ stashes.length }}</span>
      </button>
      <button
        class="stash-new"
        :disabled="!hasChanges || state.busy !== null"
        :title="hasChanges ? 'Stash changes' : 'No changes to stash'"
        :aria-label="hasChanges ? 'Stash changes' : 'No changes to stash'"
        @click="((saving = !saving), (folded = false))"
      >
        <GitIcon name="stash" />
      </button>
    </div>

    <template v-if="!folded">
      <div v-if="saving" class="stash-form">
        <input
          v-model="message"
          class="stash-input"
          placeholder="Message (optional)"
          aria-label="Stash message"
          :disabled="state.busy !== null"
          @keydown.enter="save"
        />
        <AppCheckbox :checked="includeUntracked" @click="includeUntracked = !includeUntracked">
          Include untracked files
        </AppCheckbox>
        <AppButton variant="primary" :disabled="state.busy !== null" @click="save">Stash</AppButton>
      </div>
      <p v-if="error" class="stash-error">{{ error }}</p>
      <p v-if="stashes.length === 0" class="stash-empty">No stashes</p>
      <ul v-else class="stash-list">
        <li v-for="stash in stashes" :key="stash.ref" class="stash-item">
          <button class="stash-row" :title="stash.message" @click="viewing = stash">
            <span class="stash-message">{{ stash.message }}</span>
            <span class="stash-age">{{ formatAge(stash.time) }}</span>
          </button>
          <div class="stash-actions">
            <button
              title="Apply and keep the stash"
              aria-label="Apply and keep the stash"
              :disabled="state.busy !== null"
              @click="perform('apply', stash.ref)"
            >
              <GitIcon name="apply" />
            </button>
            <button
              title="Apply and remove the stash"
              aria-label="Apply and remove the stash"
              :disabled="state.busy !== null"
              @click="perform('pop', stash.ref)"
            >
              <GitIcon name="pop" />
            </button>
            <button
              class="drop"
              :class="{ confirming: confirmingDrop === stash.ref }"
              :title="confirmingDrop === stash.ref ? 'Click again to drop' : 'Drop the stash'"
              :aria-label="confirmingDrop === stash.ref ? 'Click again to drop' : 'Drop the stash'"
              :disabled="state.busy !== null"
              @click="drop(stash)"
              @blur="confirmingDrop = null"
            >
              <GitIcon name="drop" />
            </button>
          </div>
        </li>
      </ul>
    </template>

    <AppModal v-if="viewing" size="lg" flush :title="viewing.message" @close="viewing = null">
      <div class="stash-view">
        <RevisionView :project="project" :revision="{ kind: 'stash', ref: viewing.ref }" />
      </div>
    </AppModal>
  </div>
</template>

<style scoped>
.stash-section {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.stash-head {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0 var(--space-2);
}

.stash-toggle {
  display: flex;
  align-items: center;
  gap: 6px;
  padding: 0;
  background: none;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  cursor: pointer;
}

.git-heading {
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.stash-chevron {
  transition: transform var(--duration-fast) var(--ease-out);
}

.stash-chevron.open {
  transform: rotate(90deg);
}

.stash-count {
  min-width: 16px;
  padding: 0 5px;
  border-radius: var(--radius-pill);
  background: var(--color-info);
  color: var(--color-white);
  font-size: 10px;
  font-weight: var(--weight-bold);
  line-height: 16px;
  text-align: center;
}

.stash-new {
  display: flex;
  padding: 0;
  background: none;
  border: none;
  color: var(--color-fg-muted);
  cursor: pointer;
}

.stash-new:hover:not(:disabled),
.stash-new:focus-visible:not(:disabled) {
  color: var(--color-fg-accent);
}

.stash-new:disabled {
  opacity: 0.5;
  cursor: default;
}

.stash-form {
  display: flex;
  flex-direction: column;
  gap: var(--space-2);
  padding: 0 var(--space-2);
}

.stash-input {
  box-sizing: border-box;
  width: 100%;
  padding: var(--space-1) var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
}

.stash-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.stash-error {
  max-height: 120px;
  overflow-y: auto;
  padding: var(--space-2);
  background: var(--color-error-muted);
  border-radius: var(--radius-md);
  color: var(--color-error);
  font-size: var(--text-xs);
  white-space: pre-wrap;
  word-break: break-word;
}

.stash-empty {
  padding: 0 var(--space-2);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.stash-list {
  max-height: 160px;
  overflow-y: auto;
  list-style: none;
}

.stash-item {
  padding: 2px var(--space-2);
}

.stash-row {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: 2px 0;
  background: none;
  border: none;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  text-align: left;
  cursor: pointer;
}

.stash-row:hover {
  color: var(--color-fg-primary);
}

.stash-message {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.stash-age {
  flex-shrink: 0;
  color: var(--color-fg-muted);
}

.stash-actions {
  display: flex;
  gap: var(--space-3);
}

.stash-actions button {
  display: flex;
  padding: 0;
  background: none;
  border: none;
  color: var(--color-fg-muted);
  cursor: pointer;
}

.stash-actions button:hover:not(:disabled),
.stash-actions button:focus-visible:not(:disabled) {
  color: var(--color-fg-accent);
}

.stash-actions button:disabled {
  opacity: 0.5;
  cursor: default;
}

.stash-actions .drop:hover:not(:disabled),
.stash-actions .drop:focus-visible:not(:disabled),
.stash-actions .drop.confirming {
  color: var(--color-error);
}

.stash-view {
  display: flex;
  height: 60vh;
}
</style>
