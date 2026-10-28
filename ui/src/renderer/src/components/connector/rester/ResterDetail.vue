<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onMounted, ref, toRef } from 'vue'
import { vaultSession, describeError } from '../../../vaultSession'
import type { VaultRester } from '../../../../../preload/types'
import AppButton from '../../AppButton.vue'
import EnvironmentsStrip from './EnvironmentsStrip.vue'
import ApiSection from './ApiSection.vue'
import WebSection from './WebSection.vue'
import AppSection from './AppSection.vue'
import UserCenter from './UserCenter.vue'
import { plainCopy } from './resterDraft'

const props = defineProps<{
  name: string
}>()

const emit = defineEmits<{
  close: []
}>()

const sessionPassphrase = toRef(vaultSession, 'passphrase')

const rester = ref<VaultRester | null>(null)

// Go encodes an empty slice as null, so the lists every child iterates are normalized once here.
function normalize(r: VaultRester): VaultRester {
  return { ...r, Environments: r.Environments ?? [], Users: r.Users ?? [] }
}
const loading = ref(true)
const loadError = ref('')

async function load(): Promise<void> {
  loading.value = true
  loadError.value = ''
  try {
    rester.value = normalize(await window.api.vault.getRester(sessionPassphrase.value, props.name))
  } catch (err) {
    loadError.value = describeError(err)
  } finally {
    loading.value = false
  }
}

// A rester is one encrypted record, so every edit rereads it, applies the change and writes the whole record.
async function commit(mutate: (r: VaultRester) => void): Promise<string> {
  try {
    const fresh = normalize(await window.api.vault.getRester(sessionPassphrase.value, props.name))
    mutate(fresh)
    const next = plainCopy(fresh)
    await window.api.vault.updateRester(sessionPassphrase.value, props.name, next)
    rester.value = next
    return ''
  } catch (err) {
    return describeError(err)
  }
}

onMounted(load)
</script>

<template>
  <div class="section rester-detail">
    <div class="section-toolbar">
      <AppButton @click="emit('close')">All resters</AppButton>
      <h4 class="inline-section-title rester-title">{{ name }}</h4>
    </div>

    <p v-if="loadError" class="error-text">{{ loadError }}</p>
    <p v-else-if="loading" class="empty">Loading...</p>

    <template v-else-if="rester">
      <EnvironmentsStrip :rester="rester" :commit="commit" />
      <ApiSection :rester="rester" :commit="commit" />
      <WebSection :rester="rester" :commit="commit" />
      <AppSection :rester="rester" :commit="commit" />
      <UserCenter :rester="rester" :commit="commit" />
    </template>
  </div>
</template>

<style scoped>
.rester-title {
  flex: 1;
  font-size: var(--text-md);
}
</style>
