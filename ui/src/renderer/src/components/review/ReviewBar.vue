<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import AppButton from '../AppButton.vue'
import AppSwitcher from '../AppSwitcher.vue'
import { clearReview, prepareReview, reviewOf, setReviewMessage } from '../../annotations'
import { reviewTargetsOf } from '../../reviewTargets'
import { plainCopy } from '../connector/rester/resterDraft'
import type { ProjectRef } from '../../workspace'
import type { TabBusy } from '../../../../preload/types'
import AppInput from '../AppInput.vue'

const props = defineProps<{
  project: ProjectRef
  scope: string
}>()

// Each poll runs lgrassd once, so this is as often as the picker can afford.
const BUSY_POLL_MS = 2500

const review = computed(() => reviewOf(props.project.id, props.scope))
const count = computed(() => review.value.annotations.length)
const busy = ref<Record<string, TabBusy>>({})
const targetId = ref<string | null>(null)
const sending = ref(false)
const error = ref<string | null>(null)

const targets = computed(() =>
  reviewTargetsOf(props.project.id).map((target) => ({
    id: target.tabId,
    label: target.label,
    disabled: busy.value[target.tabId]?.busy
      ? (busy.value[target.tabId].reason ?? 'Busy')
      : undefined
  }))
)

const target = computed(() => targets.value.find((t) => t.id === targetId.value) ?? null)

const blocker = computed(() => {
  if (count.value === 0 && !review.value.message.trim()) {
    return 'Add a comment or an overall comment first.'
  }
  if (!target.value) return 'Pick a tab to send the review to.'
  return target.value.disabled ?? null
})

const message = computed({
  get: () => review.value.message,
  set: (value: string) => setReviewMessage(props.project.id, props.scope, value)
})

let pollTimer: ReturnType<typeof setInterval> | null = null

async function pollBusy(): Promise<void> {
  try {
    busy.value = await window.api.tabSessions.states(props.project.path)
  } catch {
    busy.value = {}
  }
}

onMounted(() => {
  void pollBusy()
  pollTimer = setInterval(pollBusy, BUSY_POLL_MS)
})

onBeforeUnmount(() => {
  if (pollTimer) clearInterval(pollTimer)
})

async function submit(): Promise<void> {
  if (blocker.value || !targetId.value) return
  sending.value = true
  error.value = null
  try {
    const prepared = await prepareReview(props.project, props.scope)
    if (!prepared.ok) {
      error.value = prepared.error
      return
    }
    const result = await window.api.review.submit(props.project.path, targetId.value, {
      scope: plainCopy(prepared.scope),
      annotations: plainCopy(review.value.annotations),
      message: review.value.message.trim()
    })
    if (result.ok) clearReview(props.project.id, props.scope)
    else error.value = result.reason
  } catch (err) {
    error.value = err instanceof Error ? err.message : String(err)
  } finally {
    sending.value = false
  }
}
</script>

<template>
  <div class="review-bar">
    <span class="count">{{ count === 1 ? '1 comment' : `${count} comments` }}</span>
    <AppInput
      v-model="message"
      size="sm"
      class="message"
      type="text"
      spellcheck="false"
      placeholder="Overall comment"
      :disabled="sending"
    />
    <div class="picker">
      <AppSwitcher
        :items="targets"
        :active-id="targetId"
        placeholder="Send to tab"
        empty-text="No Claude or Codex tab is open in this project."
        framed
        above
        @select="targetId = $event"
      />
    </div>
    <AppButton
      variant="primary"
      :disabled="!!blocker || sending"
      :title="blocker ?? undefined"
      @click="submit"
    >
      {{ sending ? 'Sending' : 'Submit review' }}
    </AppButton>
    <p v-if="error" class="error">{{ error }}</p>
  </div>
</template>

<style scoped>
.review-bar {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-1);
  border-top: 1px solid var(--color-border-default);
}

.count {
  flex-shrink: 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-xs);
  font-weight: var(--weight-medium);
}

.message {
  flex: 1;
  min-width: 160px;
}

.picker {
  width: 220px;
}

.error {
  flex-basis: 100%;
  color: var(--color-error);
  font-size: var(--text-xs);
}
</style>
