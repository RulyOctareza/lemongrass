<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The card for a command ask. Execute runs the command in the vault daemon as the user, in the
// ask's folder; the daemon refuses anything on its never-execute list whatever this card shows.
// While it runs, the card shows the command's terminal and the human can type into it.
import AppIcon from '../AppIcon.vue'
import { computed, nextTick, onBeforeUnmount, ref, watch } from 'vue'
import type { Ask } from '../../../../preload/types'
import { describeError, vaultSession } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppConfirmButton from '../AppConfirmButton.vue'
import AppInfoPopover from '../AppInfoPopover.vue'
import AppProgressLine from '../AppProgressLine.vue'
import AppSegmented from '../AppSegmented.vue'
import AppTerminal from '../AppTerminal.vue'
import AppTextarea from '../AppTextarea.vue'
import { noteAskAnswered } from '../../notifications'
import { formatElapsed } from '../../elapsed'

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

// Set when Execute is confirmed and held until the refetched ask has left waiting, so the card
// goes straight from Execute into its running view instead of showing Execute again meanwhile.
const starting = ref(false)

const waiting = computed(() => props.ask.state === 'waiting' && !starting.value)
const running = computed(() => props.ask.state === 'running' || starting.value)
const result = computed(() => props.ask.result ?? null)

watch(
  () => props.ask.state,
  (state) => {
    if (state !== 'waiting') starting.value = false
  }
)

// The elapsed counter is the running view's clock; it ticks once a second only while running.
const now = ref(Date.now())
let ticker: ReturnType<typeof setInterval> | undefined

watch(
  running,
  (on) => {
    clearInterval(ticker)
    ticker = undefined
    if (!on) return
    now.value = Date.now()
    ticker = setInterval(() => (now.value = Date.now()), 1000)
  },
  { immediate: true }
)

onBeforeUnmount(() => clearInterval(ticker))

const startedMs = computed(() => {
  const at = props.ask.state === 'running' ? result.value?.started_at : undefined
  const ms = at ? new Date(at).getTime() : NaN
  return Number.isNaN(ms) ? null : ms
})

const elapsed = computed(() =>
  startedMs.value === null ? null : formatElapsed(now.value - startedMs.value)
)

// The daemon holds one view of a running command's terminal at a time. This card attaches while
// the command runs, and the output so far is replayed into the terminal on every attach.
const terminal = ref<InstanceType<typeof AppTerminal> | null>(null)
const live = computed(() => props.ask.state === 'running')
let attachId: number | null = null
let attaching = false
let termSize: { rows: number; cols: number } | null = null
// Set when the terminal is not showing here: another view took it over, or the connection closed.
const viewNote = ref('')

async function attachTerminal(): Promise<void> {
  if (attachId !== null || attaching || !live.value) return
  attaching = true
  viewNote.value = ''
  try {
    const view = await window.api.ask.termAttach(vaultSession.passphrase, props.ask.id)
    if (!live.value) {
      window.api.ask.termDetach(view.attachId)
      return
    }
    attachId = view.attachId
    terminal.value?.reset()
    terminal.value?.write(view.replay)
    if (termSize) window.api.ask.termResize(attachId, termSize.rows, termSize.cols)
    terminal.value?.focus()
  } catch (err) {
    // A command that has just ended refuses the attach; the refetch then shows its result.
    if (live.value) viewNote.value = describeError(err)
  } finally {
    attaching = false
  }
}

function detachTerminal(): void {
  if (attachId !== null) window.api.ask.termDetach(attachId)
  attachId = null
}

function sendInput(data: string): void {
  if (attachId !== null) window.api.ask.termInput(attachId, data)
}

// The terminal reports its size once its font is ready; past the font wait the daemon's default size
// is used, and the terminal sends its own on attach.
const SIZE_WAIT_MS = 4000
let sizeWaiter: ((size: { rows: number; cols: number }) => void) | null = null

function sendResize(size: { rows: number; cols: number }): void {
  termSize = size
  sizeWaiter?.(size)
  sizeWaiter = null
  if (attachId !== null) window.api.ask.termResize(attachId, size.rows, size.cols)
}

function terminalSize(): Promise<{ rows: number; cols: number } | null> {
  if (termSize) return Promise.resolve(termSize)
  return new Promise((resolve) => {
    sizeWaiter = resolve
    setTimeout(() => {
      if (sizeWaiter === resolve) sizeWaiter = null
      resolve(null)
    }, SIZE_WAIT_MS)
  })
}

const stopTermData = window.api.ask.onTermData(({ attachId: id, data }) => {
  if (id === attachId) terminal.value?.write(data)
})

const stopTermEnd = window.api.ask.onTermEnd(({ attachId: id, reason }) => {
  if (id !== attachId) return
  attachId = null
  if (!live.value || reason === 'ended') return
  // A view that fell behind attaches again and redraws from the replay.
  if (reason === 'behind') {
    void attachTerminal()
    return
  }
  viewNote.value =
    reason === 'replaced'
      ? 'The terminal is showing in another view.'
      : 'The terminal connection closed.'
})

watch(
  live,
  (on) => {
    if (on) void nextTick(attachTerminal)
    else detachTerminal()
  },
  { immediate: true }
)

onBeforeUnmount(() => {
  detachTerminal()
  stopTermData()
  stopTermEnd()
})

// Which Claude or Codex account a login in the command would reach, named by its folder.
const accountLines = computed(() => {
  const env = props.ask.account_env ?? {}
  const lines: { label: string; dir: string }[] = []
  if (env.CLAUDE_CONFIG_DIR) lines.push({ label: 'Claude account', dir: env.CLAUDE_CONFIG_DIR })
  if (env.CODEX_HOME) lines.push({ label: 'Codex account', dir: env.CODEX_HOME })
  return lines
})

// The rule hint's card shows while its info icon is hovered or focused.
const ruleHintAnchor = ref<HTMLElement | null>(null)

function showRuleHint(event: Event): void {
  ruleHintAnchor.value = event.currentTarget as HTMLElement
}

function hideRuleHint(): void {
  ruleHintAnchor.value = null
}

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
  starting.value = true
  void run(async () => {
    try {
      // The terminal mounts as soon as the card starts, so the command can begin at its size.
      const size = await terminalSize()
      await window.api.ask.execute(
        vaultSession.passphrase,
        props.ask.id,
        timeoutSeconds.value,
        size?.rows,
        size?.cols
      )
    } catch (err) {
      starting.value = false
      throw err
    }
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
      <pre class="untrusted mono" :class="{ running }">{{ ask.command }}</pre>
      <div v-if="running" class="run-status">
        <AppProgressLine class="run-line" label="The command is running" />
        <span
          class="run-clock"
          :title="result?.started_at ? `Started at ${clock(result.started_at)}` : undefined"
        >
          <template v-if="elapsed === null">Starting...</template>
          <template v-else
            >Running <span class="run-time">{{ elapsed }}</span></template
          >
        </span>
      </div>
      <span v-if="running && elapsed !== null" class="run-since">
        Since {{ clock(result?.started_at) }}
      </span>
      <p v-for="line in accountLines" :key="line.label" class="hint account-line">
        {{ line.label }}: <span class="account-dir">{{ line.dir }}</span>
      </p>
    </div>
    <div v-if="running" class="field-block">
      <span class="field-label">Terminal</span>
      <AppTerminal
        ref="terminal"
        label="The command's terminal"
        @data="sendInput"
        @resize="sendResize"
      />
      <p v-if="viewNote" class="hint view-note">
        {{ viewNote }}
        <AppButton :disabled="busy" @click="attachTerminal">Show here</AppButton>
      </p>
    </div>
    <div v-if="ask.reason" class="field-block">
      <span class="field-label">Reason</span>
      <pre class="untrusted">{{ ask.reason }}</pre>
    </div>
    <p v-if="!ask.verdict" class="hint">
      No guard rule matches the command. The requester asked on its own.
    </p>
    <!-- The icon uses the vault login's info-trigger look from vault.css. -->
    <p v-else class="hint guard-line">
      {{ guardLead }}<strong class="rule-id">{{ ask.verdict.rule }}</strong
      ><span
        class="info-trigger rule-help"
        tabindex="0"
        aria-label="About the rule"
        :aria-describedby="ruleHintAnchor ? 'ask-rule-help' : undefined"
        @mouseenter="showRuleHint"
        @mouseleave="hideRuleHint"
        @focus="showRuleHint"
        @blur="hideRuleHint"
        ><AppIcon name="info" :size="14" /></span
      >.
      <AppInfoPopover id="ask-rule-help" :anchor="ruleHintAnchor"
        >You can change rule {{ ask.verdict.rule }} for this project in Settings,
        Safety.</AppInfoPopover
      >
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
      <p v-if="error" class="error-text">{{ error }}</p>
      <div class="create-actions">
        <AppConfirmButton variant="danger" :disabled="busy || starting" @confirm="stop">
          Stop
        </AppConfirmButton>
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
  margin-left: 4px;
  vertical-align: -2px;
}

/* While the command runs its block's border breathes in the running tone, slowly. */
.untrusted.running {
  border-color: var(--color-jade-dim);
  animation: command-breathe 2.4s ease-in-out infinite alternate;
}

@keyframes command-breathe {
  from {
    box-shadow: 0 0 0 0 var(--color-jade-muted);
  }
  to {
    box-shadow: 0 0 10px 1px var(--color-jade-muted);
  }
}

.run-status {
  display: flex;
  align-items: center;
  gap: var(--space-3);
  margin-top: var(--space-1);
}

.run-line {
  flex: 1;
  min-width: 0;
}

.run-clock {
  flex-shrink: 0;
  color: var(--color-jade);
  font-size: var(--text-sm);
  font-weight: var(--weight-semibold);
  font-variant-numeric: tabular-nums;
}

.view-note {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: var(--space-2);
  margin: 0;
}

.account-line {
  margin: 0;
}

.account-dir {
  color: var(--color-fg);
  font-family: var(--font-mono);
  overflow-wrap: anywhere;
}

/* The body font has no tabular digits, so the time is set in mono, where every digit is one width
   and the line beside it keeps still while the counter ticks. */
.run-time {
  font-family: var(--font-mono);
}

.run-since {
  align-self: flex-end;
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

@media (prefers-reduced-motion: reduce) {
  .untrusted.running {
    animation: none;
  }
}
</style>
