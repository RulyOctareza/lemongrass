<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import AppModal from './AppModal.vue'
import { GUIDE_CARDS } from '../guide'

const emit = defineEmits<{
  close: []
}>()
</script>

<template>
  <AppModal scope="app" size="lg" title="Guide" @close="emit('close')">
    <!-- AppModal's body does not scroll by itself, so the scrolling box is here. -->
    <div class="guide-scroll">
      <p class="intro">What each part of Lemongrass is for, and when to reach for it.</p>
      <ul class="cards">
        <li v-for="card in GUIDE_CARDS" :key="card.id" class="card">
          <h4 class="card-title">{{ card.title }}</h4>
          <p class="where">{{ card.where }}</p>
          <p class="purpose">{{ card.purpose }}</p>
          <p class="line"><span class="label">When</span> {{ card.when }}</p>
          <p class="line"><span class="label">Example</span> {{ card.example }}</p>
        </li>
      </ul>
    </div>
  </AppModal>
</template>

<style scoped>
.guide-scroll {
  flex: 1;
  min-height: 0;
  overflow-y: auto;
  padding-right: var(--space-2);
}

.intro {
  margin: 0 0 var(--space-3);
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
}

.cards {
  list-style: none;
  margin: 0;
  padding: 0;
  display: flex;
  flex-direction: column;
  gap: var(--space-3);
}

.card {
  padding: var(--space-3);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-xl);
  background: var(--color-surface-2);
}

.card-title {
  margin: 0;
  font-family: var(--font-display);
  font-weight: var(--weight-semibold);
  font-size: var(--text-md);
  color: var(--color-fg-primary);
}

.where {
  margin: var(--space-1) 0 var(--space-2);
  color: var(--color-fg-muted);
  font-size: var(--text-xs);
}

.purpose,
.line {
  margin: var(--space-1) 0 0;
  color: var(--color-fg-secondary);
  font-size: var(--text-sm);
  line-height: var(--leading-relaxed);
}

.label {
  color: var(--color-fg-primary);
  font-weight: var(--weight-medium);
}
</style>
