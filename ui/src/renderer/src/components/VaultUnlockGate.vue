<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The passphrase gate shown while the vault session is locked. Its styles come from vault.css, so it
// must render inside a .vault-tab element.
import { computed, onMounted, ref } from 'vue'
import '../vault.css'
import { clearShortIds, describeError, unlockVaultSession } from '../vaultSession'
import AppButton from './AppButton.vue'

const unlockInput = ref('')
const unlockError = ref('')
const unlocking = ref(false)

const hasPassphrase = ref(false)
const hasPassphraseLoading = ref(true)
const unlockMode = computed(() => (hasPassphrase.value ? 'enter' : 'set'))

async function refreshHasPassphrase(): Promise<void> {
  hasPassphraseLoading.value = true
  try {
    hasPassphrase.value = await window.api.vault.hasPassphrase()
  } catch (err) {
    unlockError.value = describeError(err)
  } finally {
    hasPassphraseLoading.value = false
  }
}

onMounted(refreshHasPassphrase)

async function unlockVault(): Promise<void> {
  if (!unlockInput.value || unlocking.value) return
  unlocking.value = true
  unlockError.value = ''
  const passphrase = unlockInput.value
  try {
    if (unlockMode.value === 'set') {
      await window.api.vault.setPassphrase(passphrase)
      hasPassphrase.value = true
    } else {
      await window.api.vault.verifyPassphrase(passphrase)
    }
    unlockInput.value = ''
    unlockVaultSession(passphrase)
  } catch (err) {
    unlockError.value = describeError(err)
  } finally {
    unlocking.value = false
  }
}

const showResetConfirm = ref(false)
const resetting = ref(false)
const resetError = ref('')

function openResetConfirm(): void {
  showResetConfirm.value = true
  resetError.value = ''
}

function cancelResetConfirm(): void {
  showResetConfirm.value = false
}

async function confirmReset(): Promise<void> {
  if (resetting.value) return
  resetting.value = true
  resetError.value = ''
  try {
    await window.api.vault.resetVault()
    showResetConfirm.value = false
    unlockError.value = ''
    unlockInput.value = ''
    hasPassphrase.value = false
    clearShortIds()
  } catch (err) {
    resetError.value = describeError(err)
  } finally {
    resetting.value = false
  }
}
</script>

<template>
  <div class="vault-gate">
    <div v-if="!hasPassphraseLoading" class="unlock-panel">
      <template v-if="!showResetConfirm">
        <div class="unlock-head">
          <h4 class="inline-section-title">
            {{ unlockMode === 'set' ? 'Set your vault passphrase' : 'Enter your vault passphrase' }}
          </h4>
          <span class="info-trigger" tabindex="0" aria-label="About the vault">
            <svg
              width="14"
              height="14"
              viewBox="0 0 16 16"
              fill="none"
              stroke="currentColor"
              stroke-width="1.4"
              stroke-linecap="round"
              aria-hidden="true"
            >
              <circle cx="8" cy="8" r="6.5" />
              <path d="M8 7.2V11.2" />
              <circle cx="8" cy="4.9" r="0.4" fill="currentColor" />
            </svg>
          </span>
          <p class="info-popover" role="tooltip">
            <template v-if="unlockMode === 'set'">
              The vault is more than a place for credentials. It guards the parts of lemongrass that
              agents can't reach. This becomes its passphrase. Use the same one every time.
            </template>
            <template v-else>
              The vault is more than a place for credentials. It is where you run the parts of
              lemongrass that agents can't reach: connections, channel actions, safety rules and
              approvals. Enter your passphrase to access it. Auto-locks after 5 minutes idle.
            </template>
          </p>
        </div>
        <div class="unlock-row">
          <input
            v-model="unlockInput"
            type="password"
            placeholder="Vault passphrase"
            autofocus
            @keydown.enter="unlockVault"
          />
          <AppButton variant="primary" :disabled="!unlockInput || unlocking" @click="unlockVault">
            {{ unlocking ? 'Checking...' : 'Unlock' }}
          </AppButton>
        </div>
        <p v-if="unlockError" class="error-text">{{ unlockError }}</p>
        <AppButton v-if="unlockMode === 'enter'" class="reset-link" @click="openResetConfirm">
          Forgot your passphrase?
        </AppButton>
      </template>

      <template v-else>
        <h4 class="inline-section-title">Reset the vault</h4>
        <p class="hint">
          This deletes every saved connection and channel permanently. This can't be undone.
        </p>
        <p v-if="resetError" class="error-text">{{ resetError }}</p>
        <div class="unlock-row">
          <AppButton @click="cancelResetConfirm">Cancel</AppButton>
          <AppButton variant="danger" :disabled="resetting" @click="confirmReset">
            {{ resetting ? 'Resetting...' : 'Reset vault' }}
          </AppButton>
        </div>
      </template>
    </div>
  </div>
</template>
