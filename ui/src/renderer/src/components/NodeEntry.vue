<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 Faizal Alviansyah -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import NodeModal from './NodeModal.vue'

type Phase = 'checking' | 'connecting' | 'shared' | 'off'

const phase = ref<Phase>('checking')
const open = ref(false)
let timer: ReturnType<typeof setTimeout> | undefined

const view = computed(() => {
  switch (phase.value) {
    case 'checking':
      return { text: 'Checking', dot: 'Checking' }
    case 'connecting':
      return { text: 'Connecting', dot: 'Connecting' }
    case 'shared':
      return { text: 'Shared', dot: 'Connected' }
    default:
      return { text: 'Off', dot: 'Disconnected' }
  }
})

// Polls quickly while ngrok is connecting, which takes seconds, so Connecting does not linger.
async function refresh(): Promise<void> {
  clearTimeout(timer)
  try {
    const st = (await window.api.node.state()).status
    phase.value = st.running ? 'shared' : st.connecting ? 'connecting' : 'off'
  } catch {
    phase.value = 'off'
  }
  timer = setTimeout(refresh, phase.value === 'connecting' ? 2_000 : 15_000)
}

function close(): void {
  open.value = false
  void refresh()
}

onMounted(() => void refresh())
onBeforeUnmount(() => clearTimeout(timer))
</script>

<template>
  <div class="node-panel">
    <button class="node-item" @click="open = true">
      <svg
        width="13"
        height="13"
        viewBox="0 0 14 14"
        fill="none"
        stroke="currentColor"
        stroke-width="1.2"
        stroke-linecap="round"
      >
        <circle cx="7" cy="7" r="2" />
        <circle cx="2.5" cy="3" r="1.2" />
        <circle cx="11.5" cy="3" r="1.2" />
        <circle cx="7" cy="12" r="1.2" />
        <path d="M3.5 3.8L5.5 5.8M10.5 3.8L8.5 5.8M7 9v1.8" />
      </svg>
      <span class="node-label">Node</span>
      <span class="node-state">{{ view.text }}</span>
      <span class="node-dot" :class="phase" :title="view.dot" />
    </button>
    <NodeModal v-if="open" @close="close" />
  </div>
</template>

<style scoped>
.node-panel {
  flex-shrink: 0;
  padding: 0 var(--space-2) var(--space-2);
}

.node-item {
  display: flex;
  align-items: center;
  gap: 6px;
  width: 100%;
  padding: var(--space-2) var(--space-3);
  background: var(--color-surface-2);
  border: none;
  border-radius: var(--radius-md);
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
  font-weight: var(--weight-semibold);
  cursor: pointer;
  transition: background var(--duration-fast) var(--ease-out);
}

.node-item:hover {
  background: var(--color-surface-3);
}

.node-label {
  flex-shrink: 0;
}

.node-state {
  flex: 1;
  min-width: 0;
  color: var(--color-fg-secondary);
  font-weight: var(--weight-regular);
  text-align: left;
}

.node-dot {
  flex-shrink: 0;
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-error);
}

.node-dot.shared {
  background: var(--color-success);
}

.node-dot.checking,
.node-dot.connecting {
  background: var(--color-fg-muted);
  animation: node-pulse 1.2s ease-in-out infinite;
}

@keyframes node-pulse {
  50% {
    opacity: 0.3;
  }
}
</style>
