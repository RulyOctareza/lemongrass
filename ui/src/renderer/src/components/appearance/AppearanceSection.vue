<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { onMounted } from 'vue'
import AppSegmented from '../AppSegmented.vue'
import {
  loadWindowControls,
  setWindowControlsPlacement,
  windowControls,
  type WindowControlsPlacement
} from '../../windowControls'

const placementOptions: { value: WindowControlsPlacement; label: string }[] = [
  { value: 'right', label: 'Right' },
  { value: 'left', label: 'Left, macOS style' }
]

onMounted(() => void loadWindowControls())
</script>

<template>
  <div class="appearance">
    <div class="settings-field">
      <span id="appearance-window-controls">Window controls</span>
      <AppSegmented
        :model-value="windowControls.placement"
        :options="placementOptions"
        aria-labelledby="appearance-window-controls"
        @update:model-value="setWindowControlsPlacement"
      />
    </div>
    <p class="settings-text">
      Left, macOS style puts close, minimize and maximize as three colored circles at the left of
      the header.
    </p>
  </div>
</template>

<style scoped>
.appearance {
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}
</style>
