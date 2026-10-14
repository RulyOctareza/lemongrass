<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import NodeModal from './NodeModal.vue'

const running = ref(false)
const open = ref(false)
let timer: ReturnType<typeof setInterval> | undefined

async function refresh(): Promise<void> {
  try {
    running.value = (await window.api.node.state()).status.running
  } catch {
    running.value = false
  }
}

function close(): void {
  open.value = false
  void refresh()
}

onMounted(() => {
  void refresh()
  timer = setInterval(refresh, 15_000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div class="node-panel">
    <button class="node-item" :class="{ on: running }" @click="open = true">
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
      <span class="node-state">{{ running ? 'Shared' : 'Off' }}</span>
      <span v-if="running" class="node-dot" />
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

.node-item.on {
  background: var(--color-amber-muted);
  color: var(--color-fg-accent);
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
  width: 6px;
  height: 6px;
  border-radius: 50%;
  background: var(--color-amber);
}
</style>
