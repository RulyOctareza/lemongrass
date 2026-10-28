<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, ref } from 'vue'
import type { VaultRester } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import AppSegmented from '../../AppSegmented.vue'
import AppSwitcher from '../../AppSwitcher.vue'
import { fieldKeys, type CommitRester } from './resterDraft'
import { moveStep, removeStep } from './loginSteps'
import {
  appStepDrafts,
  buildAppStep,
  newAppStep,
  type AppStepAction,
  type AppStepDraft
} from './appSteps'

const props = defineProps<{
  rester: VaultRester
  commit: CommitRester
}>()

const actionOptions: { value: AppStepAction; label: string }[] = [
  { value: 'launch', label: 'Launch' },
  { value: 'tap', label: 'Tap' },
  { value: 'fill', label: 'Fill' },
  { value: 'key', label: 'Key' },
  { value: 'wait', label: 'Wait' }
]

const open = ref(false)
const saving = ref(false)
const error = ref('')
const packageName = ref('')
const steps = ref<AppStepDraft[]>([])

const fieldItems = computed(() => fieldKeys(props.rester).map((k) => ({ id: k, label: k })))

function toggle(): void {
  if (open.value) {
    open.value = false
    return
  }
  packageName.value = props.rester.App?.Package ?? ''
  steps.value = appStepDrafts(props.rester.App?.Login ?? null)
  if (steps.value.length === 0) steps.value = [newAppStep('launch')]
  error.value = ''
  open.value = true
}

// Only the handle starts a drag, so text in a row's inputs can still be selected with the mouse.
const STEP_MIME = 'application/x-lemongrass-login-step'
const dragFrom = ref<number | null>(null)
const dropAt = ref<number | null>(null)

function onHandleDragStart(event: DragEvent, index: number): void {
  if (!event.dataTransfer) return
  event.dataTransfer.effectAllowed = 'move'
  event.dataTransfer.setData(STEP_MIME, String(index))
  const row = (event.currentTarget as HTMLElement).closest('.step-row')
  if (row) event.dataTransfer.setDragImage(row, 16, 16)
  dragFrom.value = index
}

// The drop gap is above or below the hovered row, split at its middle.
function onRowDragOver(event: DragEvent, index: number): void {
  if (dragFrom.value === null || !event.dataTransfer?.types.includes(STEP_MIME)) return
  event.preventDefault()
  event.dataTransfer.dropEffect = 'move'
  const rect = (event.currentTarget as HTMLElement).getBoundingClientRect()
  dropAt.value = event.clientY < rect.top + rect.height / 2 ? index : index + 1
}

function onRowDrop(event: DragEvent): void {
  if (dragFrom.value === null || dropAt.value === null) return
  event.preventDefault()
  moveStep(steps.value, dragFrom.value, dropAt.value)
  endDrag()
}

function endDrag(): void {
  dragFrom.value = null
  dropAt.value = null
}

// A gap right beside the dragged row moves nothing, so it shows no drop line.
function showsDropLine(gap: number): boolean {
  return (
    dragFrom.value !== null &&
    dropAt.value === gap &&
    gap !== dragFrom.value &&
    gap !== dragFrom.value + 1
  )
}

async function save(): Promise<void> {
  saving.value = true
  const login = steps.value.map(buildAppStep)
  error.value = await props.commit((r) => {
    r.App = { Package: packageName.value.trim(), Login: login.length > 0 ? login : null }
  })
  saving.value = false
  if (error.value === '') open.value = false
}

async function turnOff(): Promise<void> {
  saving.value = true
  error.value = await props.commit((r) => {
    r.App = null
  })
  saving.value = false
  if (error.value === '') open.value = false
}
</script>

<template>
  <div class="inline-section">
    <button type="button" class="settings-toggle" :aria-expanded="open" @click="toggle">
      <span class="settings-chevron" :class="{ expanded: open }">▸</span>
      App side
      <span class="status-capsule" :class="{ 'status-open': rester.App }">
        {{ rester.App ? 'On' : 'Off' }}
      </span>
      <span v-if="rester.App && !open" class="settings-summary">
        {{ (rester.App.Login ?? []).length }} login steps
      </span>
    </button>

    <div v-if="open" class="settings-body">
      <p v-if="!rester.App" class="hint">Saving turns the app side on for this rester.</p>
      <div class="field-block">
        <label class="field-label" for="app-package">Package name</label>
        <input id="app-package" v-model="packageName" type="text" placeholder="com.example.app" />
      </div>
      <p class="hint">
        The phone runs these steps in order to log a user in. Fill takes a value from the user's
        login fields, so the same steps work for every user and every environment. Values are typed
        over adb, so they must be ASCII.
      </p>

      <div
        v-for="(step, index) in steps"
        :key="step.id"
        class="user-draft step-row"
        :class="{
          dragging: dragFrom === index,
          'drop-before': showsDropLine(index),
          'drop-after': index === steps.length - 1 && showsDropLine(steps.length)
        }"
        @dragover="onRowDragOver($event, index)"
        @drop="onRowDrop"
      >
        <div class="step-head">
          <span
            class="drag-handle"
            draggable="true"
            title="Drag to reorder"
            aria-label="Drag to reorder"
            @dragstart="onHandleDragStart($event, index)"
            @dragend="endDrag"
          >
            <svg width="10" height="14" viewBox="0 0 10 14" fill="currentColor" aria-hidden="true">
              <circle cx="3" cy="3" r="1.2" />
              <circle cx="7" cy="3" r="1.2" />
              <circle cx="3" cy="7" r="1.2" />
              <circle cx="7" cy="7" r="1.2" />
              <circle cx="3" cy="11" r="1.2" />
              <circle cx="7" cy="11" r="1.2" />
            </svg>
          </span>
          <span class="step-number">{{ index + 1 }}</span>
          <AppSegmented
            v-model="step.Action"
            :options="actionOptions"
            :aria-label="`Action of step ${index + 1}`"
          />
          <span class="toolbar-fill"></span>
          <button
            type="button"
            class="chip-remove"
            title="Remove step"
            @click="removeStep(steps, index)"
          >
            ×
          </button>
        </div>

        <div class="field-grid">
          <input
            v-if="step.Action !== 'launch' && step.Action !== 'key'"
            v-model="step.Text"
            type="text"
            placeholder="Text on screen"
          />
          <input
            v-if="step.Action !== 'launch' && step.Action !== 'key'"
            v-model="step.ID"
            type="text"
            placeholder="Resource id, wins over text"
          />
        </div>
        <input
          v-if="step.Action === 'key'"
          v-model="step.Key"
          type="text"
          placeholder="back, home, enter, tab, del, or a key code"
        />
        <AppSwitcher
          v-if="step.Action === 'fill'"
          :items="fieldItems"
          :active-id="step.Field || null"
          :label="step.Field || undefined"
          placeholder="Pick the user field to type"
          empty-text="No user has login fields yet."
          framed
          @select="(id) => (step.Field = id)"
        />
        <input
          v-if="step.Action !== 'launch' && step.Action !== 'key'"
          v-model.number="step.TimeoutSeconds"
          type="number"
          min="0"
          title="Timeout in seconds, 0 for the default of 10"
          placeholder="Timeout seconds, 10 by default"
        />
      </div>

      <AppButton class="new-channel-button" @click="steps.push(newAppStep('tap'))"
        >+ Step</AppButton
      >

      <div class="create-actions">
        <p v-if="error" class="error-text">{{ error }}</p>
        <AppButton v-if="rester.App" variant="danger" :disabled="saving" @click="turnOff">
          Turn off
        </AppButton>
        <AppButton @click="open = false">Cancel</AppButton>
        <AppButton variant="primary" :disabled="saving" @click="save">
          {{ saving ? 'Saving...' : 'Save' }}
        </AppButton>
      </div>
    </div>
  </div>
</template>

<style scoped>
.step-head {
  display: flex;
  align-items: center;
  gap: var(--space-2);
}

.step-row {
  position: relative;
}

.step-row.dragging {
  opacity: 0.5;
}

/* A row is followed by its own bottom margin and the list's gap, and the drop line sits midway. */
.step-row.drop-before::before,
.step-row.drop-after::after {
  content: '';
  position: absolute;
  left: 0;
  right: 0;
  height: 2px;
  border-radius: var(--radius-pill);
  background: var(--color-amber);
  pointer-events: none;
}

.step-row.drop-before::before {
  top: calc(-1 * (var(--space-2) + var(--space-3)) / 2 - 1px);
}

.step-row.drop-after::after {
  bottom: calc(-1 * (var(--space-2) + var(--space-3)) / 2 - 1px);
}

.drag-handle {
  display: flex;
  align-items: center;
  padding: 2px;
  color: var(--color-fg-muted);
  cursor: grab;
}

.drag-handle:hover {
  color: var(--color-fg-primary);
}

.drag-handle:active {
  cursor: grabbing;
}

.step-number {
  min-width: 1.5em;
  color: var(--color-fg-muted);
  font-family: var(--font-mono);
  font-size: var(--text-xs);
}

.toolbar-fill {
  flex: 1;
}
</style>
