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

const emit = defineEmits<{
  close: []
}>()

const newName = ref('')
const addError = ref('')
const adding = ref(false)
const confirmingRemoval = ref<string | null>(null)
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
  if (confirmingRemoval.value !== name) {
    confirmingRemoval.value = name
    removeError.value = ''
    return
  }
  confirmingRemoval.value = null
  removeError.value = await removeAccount(name)
}
</script>

<template>
  <AppModal scope="app" size="md" flush label="Settings" @close="emit('close')">
    <div class="settings-layout">
      <aside class="section-list">
        <h3 class="list-title">Settings</h3>
        <button type="button" class="section-item selected">Claude accounts</button>
      </aside>

      <section class="section-body">
        <h4 class="section-title">Claude accounts</h4>
        <p class="section-text">
          Each account has its own login. A new Claude tab asks which account it should run under.
        </p>

        <ul class="account-rows">
          <li class="account-row">
            <span class="account-name">primary</span>
            <span class="account-dir">{{ PRIMARY_ACCOUNT_DIR }}</span>
            <span class="primary-badge">Primary</span>
          </li>
          <li
            v-for="account in accountStore.items"
            :key="account.name"
            class="account-row"
            @mouseleave="confirmingRemoval = null"
          >
            <span class="account-name">{{ account.name }}</span>
            <span class="account-dir">{{ account.dir }}</span>
            <button
              type="button"
              class="remove-button"
              :class="{ confirming: confirmingRemoval === account.name }"
              @click="onRemove(account.name)"
            >
              {{ confirmingRemoval === account.name ? 'Click again' : 'Remove' }}
            </button>
          </li>
        </ul>
        <p v-if="removeError" class="field-error">{{ removeError }}</p>

        <form class="add-form" @submit.prevent="submitAdd">
          <input
            v-model="newName"
            class="name-input"
            type="text"
            placeholder="Account name"
            maxlength="32"
            spellcheck="false"
            autocomplete="off"
            aria-label="New account name"
          />
          <AppButton type="submit" variant="primary" :disabled="!canAdd">Add account</AppButton>
        </form>
        <p v-if="nameError || addError" class="field-error">{{ nameError || addError }}</p>
      </section>
    </div>
  </AppModal>
</template>

<style scoped>
.settings-layout {
  display: flex;
  min-height: 380px;
}

.section-list {
  flex-shrink: 0;
  width: 190px;
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  padding: var(--space-6) var(--space-3);
  background: var(--color-surface-0);
  border-radius: var(--radius-xl) 0 0 var(--radius-xl);
}

.list-title {
  padding: 0 var(--space-3) var(--space-3);
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: var(--weight-semibold);
}

.section-item {
  padding: var(--space-2) var(--space-3);
  background: transparent;
  border: none;
  border-radius: var(--radius-md, 8px);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  text-align: left;
  cursor: pointer;
}

.section-item:hover {
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
}

.section-item.selected {
  background: var(--color-amber-muted);
  color: var(--color-fg-primary);
}

.section-body {
  flex: 1;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  padding: var(--space-6);
}

.section-title {
  padding-right: var(--space-6);
  color: var(--color-fg-primary);
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: var(--weight-semibold);
}

.section-text {
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.account-rows {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
  margin: 0;
  padding: 0;
  list-style: none;
}

.account-row {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 8px);
}

.account-name {
  color: var(--color-fg-primary);
  font-size: var(--text-sm);
  font-weight: var(--weight-medium);
}

.account-dir {
  flex: 1;
  min-width: 0;
  overflow: hidden;
  text-overflow: ellipsis;
  white-space: nowrap;
  color: var(--color-fg-muted);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.primary-badge {
  padding: 2px var(--space-2);
  border-radius: var(--radius-pill);
  background: var(--color-surface-3);
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.remove-button {
  min-width: 84px;
  padding: 2px var(--space-3);
  background: transparent;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-pill);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  cursor: pointer;
  transition:
    background var(--duration-fast) var(--ease-out),
    color var(--duration-fast) var(--ease-out);
}

.remove-button:hover {
  background: var(--color-surface-3);
  color: var(--color-fg-primary);
}

.remove-button.confirming,
.remove-button.confirming:hover {
  background: var(--color-error-muted);
  border-color: var(--color-error);
  color: var(--color-error);
}

.add-form {
  display: flex;
  gap: var(--space-2);
  margin-top: var(--space-2);
}

.name-input {
  flex: 1;
  min-width: 0;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-0);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md, 8px);
  color: var(--color-fg-primary);
  font-family: var(--font-mono);
  font-size: var(--text-sm);
}

.name-input:focus {
  outline: none;
  border-color: var(--color-amber);
}

.field-error {
  margin: 0;
  color: var(--color-error);
  font-size: var(--text-xs);
}
</style>
