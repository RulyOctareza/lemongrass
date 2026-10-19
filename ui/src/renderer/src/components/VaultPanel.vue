<script setup lang="ts">
import { computed, onMounted, ref, toRef } from 'vue'
import '../vault.css'
import {
  clearShortIds,
  vaultSession,
  describeError,
  lockVaultSession,
  noteVaultActivity,
  unlockVaultSession
} from '../vaultSession'
import ConnectionsSection from './connector/ConnectionsSection.vue'
import DbChannelsSection from './connector/DbChannelsSection.vue'
import DomainsSection from './connector/DomainsSection.vue'
import HttpChannelsSection from './connector/HttpChannelsSection.vue'
import AppButton from './AppButton.vue'

const props = defineProps<{
  active: boolean
}>()

const activeKind = ref<'database' | 'http'>('database')
const activeDbTab = ref<'connections' | 'channels'>('connections')
const activeHttpTab = ref<'domains' | 'httpChannels'>('domains')

const vaultUnlocked = toRef(vaultSession, 'unlocked')
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

function shown(kind: 'database' | 'http', tab: string): boolean {
  if (!props.active || activeKind.value !== kind) return false
  return (kind === 'database' ? activeDbTab.value : activeHttpTab.value) === tab
}
</script>

<template>
  <div class="vault-tab" @click.capture="noteVaultActivity" @keydown.capture="noteVaultActivity">
    <div class="vault-frame" :class="{ 'vault-frame--gate': !vaultUnlocked }">
      <div v-if="!vaultUnlocked" class="vault-gate lg-scroll">
        <div v-if="!hasPassphraseLoading" class="unlock-panel">
          <template v-if="!showResetConfirm">
            <h4 class="inline-section-title">
              {{
                unlockMode === 'set' ? 'Set your vault passphrase' : 'Enter your vault passphrase'
              }}
            </h4>
            <p class="hint">
              <template v-if="unlockMode === 'set'">
                This becomes your vault's passphrase. Use the same one every time.
              </template>
              <template v-else>
                Unlocks your connections and channel actions for this session. Auto-locks after 5
                minutes idle.
              </template>
            </p>
            <div class="unlock-row">
              <input
                v-model="unlockInput"
                type="password"
                placeholder="Vault passphrase"
                autofocus
                @keydown.enter="unlockVault"
              />
              <AppButton
                variant="primary"
                :disabled="!unlockInput || unlocking"
                @click="unlockVault"
              >
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

      <template v-else>
        <nav class="vault-nav">
          <span class="nav-group-label">Connector</span>
          <button
            class="nav-item"
            :class="{ active: activeKind === 'database' }"
            @click="activeKind = 'database'"
          >
            <svg
              class="nav-icon"
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <ellipse cx="7" cy="3.5" rx="4.5" ry="1.8" />
              <path d="M2.5 3.5v7c0 1 2 1.8 4.5 1.8s4.5-.8 4.5-1.8v-7" />
              <path d="M2.5 7c0 1 2 1.8 4.5 1.8S11.5 8 11.5 7" />
            </svg>
            Database
          </button>
          <button
            class="nav-item"
            :class="{ active: activeKind === 'http' }"
            @click="activeKind = 'http'"
          >
            <svg
              class="nav-icon"
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <circle cx="7" cy="7" r="5.5" />
              <path d="M1.5 7h11" />
              <path
                d="M7 1.5c1.7 1.6 2.5 3.4 2.5 5.5S8.7 10.9 7 12.5C5.3 10.9 4.5 9.1 4.5 7S5.3 3.1 7 1.5z"
              />
            </svg>
            HTTP
          </button>
          <div class="nav-lock">
            <span class="unlock-status">Vault unlocked</span>
            <AppButton @click="lockVaultSession">Lock</AppButton>
          </div>
        </nav>

        <div class="vault-main lg-scroll">
          <div class="vault-content">
            <div v-show="activeKind === 'database'" class="kind-pane">
              <div class="tab-row">
                <button
                  class="tab"
                  :class="{ active: activeDbTab === 'connections' }"
                  @click="activeDbTab = 'connections'"
                >
                  Connections
                </button>
                <button
                  class="tab"
                  :class="{ active: activeDbTab === 'channels' }"
                  @click="activeDbTab = 'channels'"
                >
                  Channels
                </button>
              </div>
              <ConnectionsSection
                v-show="activeDbTab === 'connections'"
                :visible="shown('database', 'connections')"
              />
              <DbChannelsSection
                v-show="activeDbTab === 'channels'"
                :visible="shown('database', 'channels')"
              />
            </div>

            <div v-show="activeKind === 'http'" class="kind-pane">
              <div class="tab-row">
                <button
                  class="tab"
                  :class="{ active: activeHttpTab === 'domains' }"
                  @click="activeHttpTab = 'domains'"
                >
                  Domains
                </button>
                <button
                  class="tab"
                  :class="{ active: activeHttpTab === 'httpChannels' }"
                  @click="activeHttpTab = 'httpChannels'"
                >
                  HTTP Channels
                </button>
              </div>
              <DomainsSection
                v-show="activeHttpTab === 'domains'"
                :visible="shown('http', 'domains')"
              />
              <HttpChannelsSection
                v-show="activeHttpTab === 'httpChannels'"
                :visible="shown('http', 'httpChannels')"
              />
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
