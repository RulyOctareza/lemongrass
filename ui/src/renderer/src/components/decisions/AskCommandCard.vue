<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The card for a command ask. Execute runs the command in the vault daemon as the user, in the
// ask's folder; the daemon refuses anything on its never-execute list whatever this card shows.
import { computed, ref } from 'vue'
import type { Ask } from '../../../../preload/types'
import { describeError, vaultSession } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppConfirmButton from '../AppConfirmButton.vue'
import AppSegmented from '../AppSegmented.vue'
import AppTextarea from '../AppTextarea.vue'

const props = defineProps<{
  ask: Ask
}>()

const emit = defineEmits<{
  changed: [ask?: Ask]
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

const waiting = computed(() => props.ask.state === 'waiting')
const running = computed(() => props.ask.state === 'running')
const result = computed(() => props.ask.result ?? null)

function clock(iso: string | undefined): string {
  if (!iso) return ''
  const at = new Date(iso)
  if (Number.isNaN(at.getTime())) return ''
  return at.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
}

const verdictText = computed(() => {
  const verdict = props.ask.verdict
  if (!verdict) return 'No guard rule matches this command.'
  const mode = verdict.mode === 'deny' ? 'deny' : 'ask'
  return `Matches the guard rule ${verdict.rule}, set to ${mode}.`
})

const resultText = computed(() => {
  const r = result.value
  if (!r) return ''
  if (r.stopped) return `Stopped at ${clock(r.ended_at)}.`
  if (r.timed_out) return `Timed out at ${clock(r.ended_at)}.`
  return `Exited with code ${r.exit_code} at ${clock(r.ended_at)}.`
})

async function run(action: () => Promise<Ask | void>): Promise<void> {
  if (busy.value) return
  busy.value = true
  error.value = ''
  try {
    const next = await action()
    emit('changed', next ?? undefined)
  } catch (err) {
    error.value = describeError(err)
  } finally {
    busy.value = false
  }
}

function execute(): void {
  void run(() =>
    window.api.ask.execute(vaultSession.passphrase, props.ask.id, timeoutSeconds.value)
  )
}

function stop(): void {
  void run(() => window.api.ask.stop(vaultSession.passphrase, props.ask.id))
}

async function decline(): Promise<void> {
  await run(() => window.api.ask.decline(vaultSession.passphrase, props.ask.id, note.value.trim()))
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
    <p class="hint">
      {{ verdictText }}
      <template v-if="ask.rule">The guard filed this request when it refused the call.</template>
    </p>

    <template v-if="waiting">
      <p v-if="ask.never_execute" class="error-text">
        Lemongrass never runs this command. {{ ask.never_execute_reason }}
      </p>
      <div v-else class="field-block">
        <span id="ask-timeout-label" class="field-label">Stop it after</span>
        <AppSegmented
          v-model="timeoutSeconds"
          aria-labelledby="ask-timeout-label"
          :options="timeoutOptions"
        />
      </div>
      <div class="field-block">
        <label class="field-label" for="ask-decline-note">Note for the agent if you decline</label>
        <AppTextarea
          id="ask-decline-note"
          v-model="note"
          size="sm"
          :rows="2"
          placeholder="Why you decline, or what to do instead"
        />
      </div>
      <p v-if="error" class="error-text">{{ error }}</p>
      <div class="create-actions">
        <AppButton :disabled="busy" @click="decline">Decline</AppButton>
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

    <template v-else-if="running">
      <p class="hint">Running since {{ clock(result?.started_at) }}.</p>
      <p v-if="error" class="error-text">{{ error }}</p>
      <div class="create-actions">
        <AppConfirmButton variant="danger" :disabled="busy" @confirm="stop">Stop</AppConfirmButton>
      </div>
    </template>

    <template v-else-if="result">
      <p class="hint">{{ resultText }} The agent has the result.</p>
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
</style>
