<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
// The grant view of a waiting rester ask: the human picks the rester, its environment and a TTL, and
// the methods the model declared come prefilled. Methods scope an API channel, so the grant is
// always an API channel. The daemon refuses a method outside the declared ones whatever this view
// allows. Renders inside a .vault-tab element.
import { computed, ref, watch } from 'vue'
import type { Ask, VaultHTTPChannelWithShortId, VaultRester } from '../../../../preload/types'
import { describeError, vaultSession } from '../../vaultSession'
import AppButton from '../AppButton.vue'
import AppCheckbox from '../AppCheckbox.vue'
import AppInput from '../AppInput.vue'
import AppSegmented from '../AppSegmented.vue'
import AppSwitcher from '../AppSwitcher.vue'

const props = defineProps<{
  ask: Ask
  resters: string[]
}>()

const emit = defineEmits<{
  created: [result: VaultHTTPChannelWithShortId]
  decline: []
}>()

// The human's pick for db asks, kept the same here: the method boxes open for unchecking only once
// the picked environment can carry an API channel. One value, so it is easy to change.
const METHODS_EDITABLE_AFTER_CHECK = true

const methodOptions = ['GET', 'POST', 'PUT', 'PATCH', 'DELETE', 'HEAD', 'OPTIONS'] as const

const declared = computed(() => (props.ask.methods ?? []).map((m) => m.toUpperCase()))

const resterName = ref('')
const environment = ref('')
const ttlMinutes = ref(10)
const methods = ref<string[]>([...declared.value])

interface ExclusionDraft {
  method: string
  pathPattern: string
}
const exclusions = ref<ExclusionDraft[]>([])

const rester = ref<VaultRester | null>(null)
const resterLoading = ref(false)
const resterError = ref('')
let loadSeq = 0

async function pickRester(name: string): Promise<void> {
  resterName.value = name
  environment.value = ''
  rester.value = null
  resterError.value = ''
  const seq = ++loadSeq
  resterLoading.value = true
  try {
    const next = await window.api.vault.getRester(vaultSession.passphrase, name)
    if (seq !== loadSeq) return
    rester.value = next
    environment.value = next.Environments?.[0]?.Name ?? ''
  } catch (err) {
    if (seq === loadSeq) resterError.value = describeError(err)
  } finally {
    if (seq === loadSeq) resterLoading.value = false
  }
}

const selectedEnvironment = computed(
  () => rester.value?.Environments?.find((e) => e.Name === environment.value) ?? null
)

// Why the picked rester and environment cannot carry an API channel, empty when they can.
const apiUnavailable = computed(() => {
  if (!rester.value) return ''
  if (!rester.value.API) return `Rester ${resterName.value} has no API side.`
  const env = selectedEnvironment.value
  if (!env) return `Rester ${resterName.value} has no environments.`
  if (!env.BaseURL) return `Environment ${env.Name} has no base URL.`
  return ''
})

const ready = computed(() => !!rester.value && !!selectedEnvironment.value && !apiUnavailable.value)

const methodsLocked = computed(() => !METHODS_EDITABLE_AFTER_CHECK || !ready.value)

watch(methodsLocked, (locked) => {
  if (locked) methods.value = [...declared.value]
})

function toggleMethod(method: string): void {
  if (methodsLocked.value) return
  const i = methods.value.indexOf(method)
  if (i === -1) methods.value.push(method)
  else methods.value.splice(i, 1)
}

function addExclusion(): void {
  exclusions.value.push({ method: methods.value[0] ?? 'GET', pathPattern: '' })
}

function removeExclusion(index: number): void {
  exclusions.value.splice(index, 1)
}

const creating = ref(false)
const createError = ref('')

const holdReason = computed(() => {
  if (declared.value.length === 0) return 'The ask declares no methods, so it can only be declined.'
  if (!resterName.value) return 'Pick a rester and its environment to approve.'
  if (resterLoading.value) return `Reading rester ${resterName.value}.`
  if (resterError.value) return ''
  if (apiUnavailable.value) return `${apiUnavailable.value} Pick another rester or environment.`
  if (methods.value.length === 0) return 'Check at least one method to approve.'
  return ''
})

const canApprove = computed(
  () =>
    ready.value &&
    declared.value.length > 0 &&
    methods.value.length > 0 &&
    ttlMinutes.value > 0 &&
    vaultSession.unlocked &&
    !creating.value
)

async function approve(): Promise<void> {
  if (!canApprove.value) return
  creating.value = true
  createError.value = ''
  try {
    const result = await window.api.vault.createHTTPChannel(
      vaultSession.passphrase,
      `ask ${props.ask.id}`,
      resterName.value,
      environment.value,
      'api',
      {
        Methods: [...methods.value],
        Exclusions: exclusions.value
          .filter((e) => e.pathPattern.trim())
          .map((e) => ({ Method: e.method, PathPattern: e.pathPattern.trim() }))
      },
      null,
      Math.round(ttlMinutes.value * 60),
      props.ask.id
    )
    emit('created', result)
  } catch (err) {
    createError.value = describeError(err)
  } finally {
    creating.value = false
  }
}
</script>

<template>
  <div class="inline-section">
    <div class="form-columns">
      <div class="form-column">
        <div class="field-block">
          <span class="field-label">Rester</span>
          <AppSwitcher
            framed
            :items="resters.map((name) => ({ id: name, label: name }))"
            :active-id="resterName || null"
            placeholder="Pick a rester"
            empty-text="No resters yet."
            @select="pickRester"
          />
          <p v-if="resterError" class="error-text">{{ resterError }}</p>
        </div>
        <div v-if="rester?.Environments?.length" class="field-block">
          <span class="field-label">Environment</span>
          <AppSegmented
            aria-label="Environment"
            :options="rester.Environments.map((e) => ({ value: e.Name, label: e.Name }))"
            :model-value="environment"
            @update:model-value="environment = $event"
          />
        </div>
        <div class="field-block">
          <label class="field-label" :for="`ask-ttl-${ask.id}`">TTL</label>
          <AppInput
            :id="`ask-ttl-${ask.id}`"
            v-model.number="ttlMinutes"
            type="number"
            min="1"
            @keydown.enter="approve"
          />
          <p class="hint">
            The channel stays open for {{ ttlMinutes }} minutes before it needs reactivating.
          </p>
        </div>
      </div>

      <div class="form-column">
        <div class="field-block">
          <span class="field-label">Methods</span>
          <div class="checkbox-grid">
            <AppCheckbox
              v-for="m in declared"
              :key="m"
              class="method-box"
              :checked="methods.includes(m)"
              :disabled="methodsLocked"
              @click="toggleMethod(m)"
            >
              {{ m }}
            </AppCheckbox>
          </div>
          <p
            v-if="METHODS_EDITABLE_AFTER_CHECK && methodsLocked && declared.length > 0"
            class="hint"
          >
            The methods stay as the requester declared them until a rester and environment with an
            API side are picked.
          </p>
        </div>
        <div class="field-block">
          <span class="field-label">Exclusions</span>
          <p class="hint">Closes one path pattern for a granted method.</p>
          <div v-for="(ex, i) in exclusions" :key="i" class="exclusion-row">
            <AppSwitcher
              framed
              :items="methodOptions.map((m) => ({ id: m, label: m }))"
              :active-id="ex.method"
              placeholder="Method"
              empty-text="No methods."
              @select="(m) => (ex.method = m)"
            />
            <AppInput v-model="ex.pathPattern" type="text" placeholder="/users/*/change-password" />
            <button
              type="button"
              class="chip-remove"
              :aria-label="`Remove exclusion ${ex.method} ${ex.pathPattern}`"
              @click="removeExclusion(i)"
            >
              ×
            </button>
          </div>
          <AppButton type="button" @click="addExclusion">+ Exclusion</AppButton>
        </div>
      </div>
    </div>

    <p v-if="createError" class="error-text">{{ createError }}</p>
    <p v-else-if="holdReason" class="hint">{{ holdReason }}</p>
    <div class="create-actions">
      <AppButton :disabled="creating" @click="emit('decline')">Decline</AppButton>
      <AppButton variant="primary" :disabled="!canApprove" @click="approve">
        {{ creating ? 'Approving...' : 'Approve' }}
      </AppButton>
    </div>
  </div>
</template>

<style scoped>
.method-box:disabled {
  cursor: default;
  opacity: 0.7;
}
</style>
