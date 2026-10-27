<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import AppModal from './AppModal.vue'
import {
  PRIMARY_ACCOUNT_DIR,
  accountNameError,
  accountStore,
  addAccount,
  removeAccount
} from '../accounts'
import AppButton from './AppButton.vue'
import AppConfirmButton from './AppConfirmButton.vue'
import PolicySection from './safety/PolicySection.vue'
import type { Project } from '../../../preload/types'

const props = defineProps<{
  projects: Project[]
  activeProjectId: string | null
}>()

const emit = defineEmits<{
  close: []
}>()

const section = ref<'accounts' | 'safety'>('accounts')

const newName = ref('')
const addError = ref('')
const adding = ref(false)
const removeError = ref('')

const nameError = computed(() => accountNameError(newName.value))
const canAdd = computed(() => newName.value.length > 0 && !nameError.value && !adding.value)

async function submitAdd(): Promise<void> {
  if (!canAdd.value) return
  adding.value = true
  addError.value = ''
  const error = await addAccount(newName.value)
  adding.value = false
  if (error) addError.value = error
  else newName.value = ''
}

async function onRemove(name: string): Promise<void> {
  removeError.value = await removeAccount(name)
}
</script>

<template>
  <AppModal scope="app" size="lg" flush label="Settings" @close="emit('close')">
    <div class="settings-layout">
      <aside class="settings-nav">
        <h3 class="settings-nav-title">Settings</h3>
        <button
          type="button"
          class="settings-nav-item"
          :class="{ selected: section === 'accounts' }"
          @click="section = 'accounts'"
        >
          Claude accounts
        </button>
        <button
          type="button"
          class="settings-nav-item"
          :class="{ selected: section === 'safety' }"
          @click="section = 'safety'"
        >
          Safety
        </button>
      </aside>

      <section v-if="section === 'safety'" class="settings-section settings-section--fixed">
        <h4 class="settings-title">Safety</h4>
        <PolicySection :projects="props.projects" :active-project-id="props.activeProjectId" />
      </section>

      <section v-else class="settings-section">
        <h4 class="settings-title">Claude accounts</h4>
        <p class="settings-text">
          Each account has its own login. A new Claude tab asks which account it should run under.
        </p>

        <ul class="settings-rows">
          <li class="settings-row">
            <span class="settings-row-title">primary</span>
            <span class="settings-row-meta">{{ PRIMARY_ACCOUNT_DIR }}</span>
            <span class="settings-badge">Primary</span>
          </li>
          <li v-for="account in accountStore.items" :key="account.name" class="settings-row">
            <span class="settings-row-title">{{ account.name }}</span>
            <span class="settings-row-meta">{{ account.dir }}</span>
            <AppConfirmButton
              v-slot="{ armed }"
              class="settings-action"
              @confirm="onRemove(account.name)"
            >
              {{ armed ? 'Click again' : 'Remove' }}
            </AppConfirmButton>
          </li>
        </ul>
        <p v-if="removeError" class="settings-error">{{ removeError }}</p>

        <form class="settings-add" @submit.prevent="submitAdd">
          <input
            v-model="newName"
            class="settings-input"
            type="text"
            placeholder="Account name"
            maxlength="32"
            spellcheck="false"
            autocomplete="off"
            aria-label="New account name"
          />
          <AppButton type="submit" variant="primary" :disabled="!canAdd">Add account</AppButton>
        </form>
        <p v-if="nameError || addError" class="settings-error">{{ nameError || addError }}</p>
      </section>
    </div>
  </AppModal>
</template>
