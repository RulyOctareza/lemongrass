<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The modal for one ask, opened from the OS notification or the Decisions section over whatever view
// is showing. A locked vault is unlocked in place first, which unlocks it for the whole app. The ask's
// texts are model output, so they are only ever interpolated as text.
import { computed, nextTick, ref, watch } from 'vue'
import '../../vault.css'
import type { Ask } from '../../../../preload/types'
import { askState, closeAsk } from '../../asks'
import { describeError, noteVaultActivity, vaultSession } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppModal from '../AppModal.vue'
import AppTextarea from '../AppTextarea.vue'
import VaultUnlockGate from '../VaultUnlockGate.vue'
import DbChannelForm from '../connector/DbChannelForm.vue'
import HttpChannelForm from '../connector/HttpChannelForm.vue'
import AskCommandCard from './AskCommandCard.vue'
import { noteAskAnswered } from '../../notifications'
import { askStateLabel, askTypeLabel } from './askLabels'
import { askParts, askWhere, loadAskProjects } from './askOrigin'
import MarkedText from '../MarkedText.vue'

const ask = ref<Ask | null>(null)
const loadError = ref('')
const connections = ref<string[]>([])
const resters = ref<string[]>([])
let loadSeq = 0

const title = computed(() => (ask.value ? askTypeLabel(ask.value.type) : 'Decision'))

loadAskProjects()

async function load(quiet: boolean): Promise<void> {
  const id = askState.openId
  if (!id || !vaultSession.unlocked) return
  const seq = ++loadSeq
  if (!quiet) {
    ask.value = null
    loadError.value = ''
  }
  try {
    const next = await window.api.ask.get(vaultSession.passphrase, id)
    if (seq !== loadSeq) return
    ask.value = next
    loadError.value = ''
    if (next.state !== 'waiting') return
    if (next.type === 'db') connections.value = await window.api.vault.listConnections()
    if (next.type === 'rester') resters.value = await window.api.vault.listResters()
  } catch (err) {
    if (seq === loadSeq) loadError.value = describeError(err)
  }
}

watch(
  () => [askState.openId, vaultSession.unlocked] as const,
  ([id, unlocked]) => {
    if (!id) ask.value = null
    else if (unlocked) void load(false)
  },
  { immediate: true }
)

watch(
  () => askState.version,
  () => void load(true)
)

function onCommandChanged(): void {
  void load(true)
}

function onDbCreated(result: { channel: { ID: string }; shortId: string }): void {
  vaultSession.shortIdByChannel[result.channel.ID] = result.shortId
  if (ask.value) noteAskAnswered(ask.value, 'granted')
  closeAsk()
}

function onHttpCreated(result: { channel: { ID: string }; shortId: string }): void {
  vaultSession.shortIdByHTTPChannel[result.channel.ID] = result.shortId
  if (ask.value) noteAskAnswered(ask.value, 'granted')
  closeAsk()
}

const note = ref('')
// Decline is a second step: the note field shows only once the human means to decline. The form
// stays mounted behind it, so Back returns to what was already filled in.
const declineStep = ref(false)
const declining = ref(false)
const declineError = ref('')
const noteInput = ref<InstanceType<typeof AppTextarea> | null>(null)

watch(
  () => askState.openId,
  () => {
    note.value = ''
    declineStep.value = false
    declineError.value = ''
  }
)

function startDecline(): void {
  declineStep.value = true
  declineError.value = ''
  void nextTick(() => noteInput.value?.focus())
}

function cancelDecline(): void {
  declineStep.value = false
  declineError.value = ''
}

async function declineChannel(): Promise<void> {
  if (!ask.value || declining.value) return
  declining.value = true
  declineError.value = ''
  try {
    await window.api.ask.decline(vaultSession.passphrase, ask.value.id, note.value.trim())
    noteAskAnswered(ask.value, 'declined')
    closeAsk()
  } catch (err) {
    declineError.value = describeError(err)
  } finally {
    declining.value = false
  }
}

function askedAt(iso: string): string {
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleString([], {
    month: 'short',
    day: 'numeric',
    hour: '2-digit',
    minute: '2-digit'
  })
}

// Waiting needs no chip: the actions below say it. A finished command reads as done too.
const status = computed(() => {
  const state = ask.value?.state
  if (state === 'answered') return { label: 'Done', tone: 'done' }
  if (state === 'running') return { label: askStateLabel(state), tone: 'running' }
  if (state === 'declined' || state === 'expired')
    return { label: askStateLabel(state), tone: 'muted' }
  return null
})

const outcome = computed(() => {
  const a = ask.value
  if (!a || a.type === 'command') return ''
  if (a.state === 'answered')
    return a.short_id
      ? `The requester has channel ${a.short_id}.`
      : 'The requester has the channel.'
  if (a.state === 'declined') return 'The requester has your decline and its note.'
  if (a.state === 'expired') return 'The ask expired with no answer.'
  return ''
})
</script>

<template>
  <AppModal v-if="askState.openId" scope="app" size="md" :title="title" @close="closeAsk">
    <div
      class="vault-tab ask-body"
      @click.capture="noteVaultActivity"
      @keydown.capture="noteVaultActivity"
    >
      <VaultUnlockGate v-if="!vaultSession.unlocked" />

      <template v-else>
        <p v-if="loadError" class="error-text">{{ loadError }}</p>
        <p v-else-if="!ask" class="empty">Loading...</p>

        <template v-else>
          <div class="ask-head">
            <div class="ask-lead-row">
              <p class="ask-lead"><MarkedText :parts="askParts(ask)" /></p>
              <span v-if="status" class="ask-status" :class="`ask-status--${status.tone}`">
                {{ status.label }}
              </span>
            </div>
            <p class="ask-meta" :title="ask.cwd">
              Asked {{ askedAt(ask.created_at) }} in {{ askWhere(ask) }}.
            </p>
          </div>

          <AskCommandCard
            v-if="ask.type === 'command'"
            :key="ask.id"
            :ask="ask"
            @changed="onCommandChanged"
            @done="closeAsk"
          />

          <template v-else>
            <div v-if="ask.type === 'rester'" class="field-block">
              <span class="field-label">Message</span>
              <pre class="untrusted">{{ ask.message }}</pre>
            </div>
            <div v-if="ask.reason" class="field-block">
              <span class="field-label">Reason</span>
              <pre class="untrusted">{{ ask.reason }}</pre>
            </div>

            <p v-if="outcome" class="hint">{{ outcome }}</p>

            <template v-else-if="ask.state === 'waiting'">
              <div v-show="!declineStep" class="channel-step">
                <p class="hint">
                  You set the permissions and how long the channel stays open. Creating the channel
                  sends its short id to the requester.
                </p>
                <template v-if="ask.type === 'db'">
                  <p v-if="connections.length === 0" class="hint">
                    Add a database connection in the Vault first.
                  </p>
                  <DbChannelForm
                    v-else
                    :key="ask.id"
                    title="New database channel"
                    :connections="connections"
                    :requested-tables="ask.tables ?? []"
                    :ask-id="ask.id"
                    @created="onDbCreated"
                    @cancel="closeAsk"
                  />
                </template>
                <template v-else>
                  <p v-if="resters.length === 0" class="hint">Add a rester in the Vault first.</p>
                  <HttpChannelForm
                    v-else
                    :key="ask.id"
                    title="New rester channel"
                    :resters="resters"
                    :ask-id="ask.id"
                    @created="onHttpCreated"
                    @cancel="closeAsk"
                  />
                </template>

                <div class="create-actions decline-block">
                  <AppButton @click="startDecline">Decline</AppButton>
                </div>
              </div>

              <div v-if="declineStep" class="field-block">
                <label class="field-label" for="ask-channel-note">Note for the requester</label>
                <AppTextarea
                  id="ask-channel-note"
                  ref="noteInput"
                  v-model="note"
                  size="sm"
                  :rows="2"
                  placeholder="Why you decline, or what to do instead"
                />
                <p v-if="declineError" class="error-text">{{ declineError }}</p>
                <div class="create-actions">
                  <AppButton :disabled="declining" @click="cancelDecline">Back</AppButton>
                  <AppButton variant="primary" :disabled="declining" @click="declineChannel">
                    {{ declining ? 'Declining...' : 'Send decline' }}
                  </AppButton>
                </div>
              </div>
            </template>
          </template>
        </template>
      </template>
    </div>
  </AppModal>
</template>

<style>
.vault-tab.ask-body {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
  overflow: visible;
}

.vault-tab.ask-body .vault-gate {
  padding: var(--space-4) 0;
}

.vault-tab .untrusted {
  max-height: 240px;
  margin: 0;
  padding: var(--space-2) var(--space-3);
  overflow: auto;
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-md);
  background: var(--color-surface-2);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  white-space: pre-wrap;
  overflow-wrap: anywhere;
}

.vault-tab .untrusted.mono {
  font-family: var(--font-mono);
}
</style>

<style scoped>
.ask-head {
  display: flex;
  flex-direction: column;
  gap: var(--space-1);
}

.ask-lead-row {
  display: flex;
  align-items: flex-start;
  justify-content: space-between;
  gap: var(--space-3);
}

.ask-status {
  flex-shrink: 0;
  padding: 2px 10px;
  border-radius: var(--radius-pill);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
}

.ask-status--done {
  background: var(--color-jade-muted);
  color: var(--color-jade);
}

.ask-status--running {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
}

.ask-status--muted {
  background: var(--color-surface-3);
  color: var(--color-fg-secondary);
}

.ask-lead {
  margin: 0;
  color: var(--color-fg-primary);
  font-size: var(--text-base);
  font-weight: var(--weight-semibold);
}

.ask-meta {
  margin: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
}

.channel-step {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.decline-block {
  padding-top: var(--space-3);
  border-top: 1px solid var(--color-border-default);
}
</style>
