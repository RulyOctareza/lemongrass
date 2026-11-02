<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The card for a command ask. Execute runs the command in the vault daemon as the user, in the
// ask's folder; the daemon refuses anything on its never-execute list whatever this card shows.
import { computed, nextTick, ref } from 'vue'
import type { Ask } from '../../../../preload/types'
import { describeError, vaultSession } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppConfirmButton from '../AppConfirmButton.vue'
import AppSegmented from '../AppSegmented.vue'
import AppTextarea from '../AppTextarea.vue'
import { noteAskAnswered } from '../../notifications'

const props = defineProps<{
  ask: Ask
}>()

const emit = defineEmits<{
  changed: []
  done: []
}>()

// The daemon's choices; it caps anything else at four hours.
const timeoutOptions = [
  { value: 600, label: '10 min' },
  { value: 3600, label: '1 hour' },
  { value: 14400, label: '4 hours' }
]

const timeoutSeconds = ref(600)
const note = ref('')
const busy = ref(false)
const error = ref('')
// Decline is a second step, so the note field shows only once the human means to decline.
const declining = ref(false)
const noteInput = ref<InstanceType<typeof AppTextarea> | null>(null)

function startDecline(): void {
  declining.value = true
  error.value = ''
  void nextTick(() => noteInput.value?.focus())
}

function cancelDecline(): void {
  declining.value = false
  error.value = ''
}

const waiting = computed(() => props.ask.state === 'waiting')
const running = computed(() => props.ask.state === 'running')
const result = computed(() => props.ask.result ?? null)

function clock(iso: string | undefined): string {
  if (!iso) return ''
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

// The part before the marked rule id; the rule id and its hint follow in the template.
const guardLead = computed(() => {
  const verdict = props.ask.verdict
  if (!verdict) return ''
  const what = verdict.description || 'guarded'
  return verdict.mode === 'deny'
    ? `The command is ${what}, which the guard denies to agents under rule `
    : `The command is ${what}, which the guard leaves to you under rule `
})

// Marked in the template; the plain rest of the line follows it.
const resultHead = computed(() => {
  const r = result.value
  if (!r) return ''
  if (r.stopped) return 'Stopped'
  if (r.timed_out) return 'Timed out'
  return `Exited with code ${r.exit_code}`
})

// The parent refetches after each action, so the card only ever shows the daemon's own record.
async function run(action: () => Promise<unknown>): Promise<void> {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    await action()
    emit('changed')
  } catch (err) {
    error.value = describeError(err)
  } finally {
    busy.value = false
  }
}

function execute(): void {
  void run(async () => {
    await window.api.ask.execute(vaultSession.passphrase, props.ask.id, timeoutSeconds.value)
    noteAskAnswered(props.ask, 'ran')
  })
}

function stop(): void {
  void run(() => window.api.ask.stop(vaultSession.passphrase, props.ask.id))
}

async function decline(): Promise<void> {
  await run(async () => {
    await window.api.ask.decline(vaultSession.passphrase, props.ask.id, note.value.trim())
    noteAskAnswered(props.ask, 'declined')
  })
  if (!error.value) emit('done')
}
</script>

<template>
  <div class="command-card">
    <div class="field-block">
      <span class="field-label">Command</span>
      <pre class="untrusted mono">{{ ask.command }}</pre>
    </div>
    <div v-if="ask.reason" class="field-block">
      <span class="field-label">Reason</span>
      <pre class="untrusted">{{ ask.reason }}</pre>
    </div>
    <p v-if="!ask.verdict" class="hint">
      No guard rule matches the command. The requester asked on its own.
    </p>
    <p v-else class="hint">
      {{ guardLead }}<strong class="rule-id">{{ ask.verdict.rule }}</strong
      ><span
        class="rule-help"
        role="img"
        aria-label="You can change this rule for this project in Settings, Safety."
        title="You can change this rule for this project in Settings, Safety."
        ><svg
          width="12"
          height="12"
          viewBox="0 0 12 12"
          fill="none"
          stroke="currentColor"
          stroke-width="1.1"
          stroke-linecap="round"
        >
          <circle cx="6" cy="6" r="5" />
          <path d="M4.6 4.6a1.4 1.4 0 1 1 1.9 1.3c-.4.2-.5.4-.5.8" />
          <circle cx="6" cy="8.6" r=".3" fill="currentColor" /></svg></span
      >.
      <template v-if="ask.rule">The guard refused it and sent it to you.</template>
    </p>

    <template v-if="waiting">
      <template v-if="!declining">
        <p v-if="ask.never_execute" class="error-text">
          Lemongrass never runs the command. {{ ask.never_execute_reason }}
        </p>
        <div v-else class="field-block">
          <span id="ask-timeout-label" class="field-label">Stop the command after</span>
          <AppSegmented
            v-model="timeoutSeconds"
            aria-labelledby="ask-timeout-label"
            :options="timeoutOptions"
          />
        </div>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="create-actions">
          <AppButton :disabled="busy" @click="startDecline">Decline</AppButton>
          <AppConfirmButton
            v-if="!ask.never_execute"
            variant="primary"
            tone="jade"
            :disabled="busy"
            @confirm="execute"
          >
            {{ busy ? 'Starting...' : 'Execute' }}
          </AppConfirmButton>
        </div>
      </template>

      <template v-else>
        <div class="field-block">
          <label class="field-label" for="ask-decline-note">Note for the requester</label>
          <AppTextarea
            id="ask-decline-note"
            ref="noteInput"
            v-model="note"
            size="sm"
            :rows="2"
            placeholder="Why you decline, or what to do instead"
          />
        </div>
        <p v-if="error" class="error-text">{{ error }}</p>
        <div class="create-actions">
          <AppButton :disabled="busy" @click="cancelDecline">Back</AppButton>
          <AppButton variant="primary" :disabled="busy" @click="decline">
            {{ busy ? 'Declining...' : 'Send decline' }}
          </AppButton>
        </div>
      </template>
    </template>

    <template v-else-if="running">
      <p class="hint">Running since {{ clock(result?.started_at) }}.</p>
      <p v-if="error" class="error-text">{{ error }}</p>
      <div class="create-actions">
        <AppConfirmButton variant="danger" :disabled="busy" @confirm="stop">Stop</AppConfirmButton>
      </div>
    </template>

    <template v-else-if="result">
      <p class="hint">
        <strong class="result-head">{{ resultHead }}</strong> at {{ clock(result.ended_at) }}. The
        requester has its response.
      </p>
      <div class="field-block">
        <span class="field-label">Last lines of output</span>
        <pre class="untrusted mono">{{ result.output_tail || 'No output.' }}</pre>
      </div>
    </template>
  </div>
</template>

<style scoped>
.command-card {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.rule-id,
.result-head {
  color: var(--color-fg-accent);
  font-weight: var(--weight-semibold);
}

.rule-id {
  font-family: var(--font-mono);
}

.rule-help {
  display: inline-flex;
  margin-left: 4px;
  color: var(--color-fg-secondary);
  vertical-align: -1px;
  cursor: help;
}
</style>
