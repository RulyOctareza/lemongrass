<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { ref } from 'vue'
import AppButton from '../AppButton.vue'
import AppTextarea from '../AppTextarea.vue'
import AppSegmented from '../AppSegmented.vue'
import AppSwitcher from '../AppSwitcher.vue'
import PairRows from './PairRows.vue'
import type { ResterClientBodyKind, ResterClientRequest } from '../../../../preload/types'
import AppEmptyState from '../AppEmptyState.vue'

// Edited in place; the parent owns the request.
const request = defineModel<ResterClientRequest>({ required: true })

const section = ref<'body' | 'headers'>('body')
const formatError = ref('')

const sections = [
  { value: 'body' as const, label: 'Body' },
  { value: 'headers' as const, label: 'Headers' }
]

const bodyKinds: { id: ResterClientBodyKind; label: string }[] = [
  { id: 'none', label: 'None' },
  { id: 'json', label: 'JSON' },
  { id: 'form', label: 'Form' }
]

function formatJson(): void {
  formatError.value = ''
  if (!request.value.json.trim()) return
  try {
    request.value.json = JSON.stringify(JSON.parse(request.value.json), null, 2)
  } catch (err) {
    formatError.value = err instanceof Error ? err.message : String(err)
  }
}
</script>

<template>
  <section class="box">
    <header class="box-head">
      <span class="box-title">Request</span>
      <AppSegmented v-model="section" :options="sections" aria-label="Request part" />
      <div v-if="section === 'body'" class="body-kind">
        <AppSwitcher
          framed
          :items="bodyKinds"
          :active-id="request.bodyKind"
          placeholder="None"
          empty-text=""
          @select="request.bodyKind = $event as ResterClientBodyKind"
        />
      </div>
      <AppButton
        v-if="section === 'body' && request.bodyKind === 'json'"
        class="format"
        @click="formatJson"
      >
        Format
      </AppButton>
    </header>

    <div class="box-body">
      <template v-if="section === 'body'">
        <AppEmptyState
          v-if="request.bodyKind === 'none'"
          layout="fill"
          title="This request sends no body."
        />
        <template v-else-if="request.bodyKind === 'json'">
          <AppTextarea
            v-model="request.json"
            class="json-input"
            size="sm"
            resize="none"
            mono
            spellcheck="false"
            placeholder="{ }"
            @input="formatError = ''"
          />
          <p v-if="formatError" class="error-text">{{ formatError }}</p>
        </template>
        <PairRows v-else v-model="request.form" add-label="Field" name-placeholder="field" />
      </template>

      <template v-else>
        <p class="hint">
          The rester's own headers and the user's token are added when the request is sent.
        </p>
        <PairRows v-model="request.headers" add-label="Header" name-placeholder="header" />
      </template>
    </div>
  </section>
</template>

<style scoped>
.body-kind {
  width: 104px;
}

.format {
  margin-left: auto;
}

.json-input {
  flex: 1;
  min-height: 220px;
  tab-size: 2;
}
</style>
