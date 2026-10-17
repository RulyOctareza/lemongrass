<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref } from 'vue'
import {
  attachShell,
  closeFailedShell,
  continueOnPrimary,
  fitShell,
  shellAccountGone,
  shellRestoreFailures,
  startFreshShell,
  type ShellSpec
} from '../shellRegistry'
import AppButton from './AppButton.vue'

const props = defineProps<{
  spec: ShellSpec
}>()

const containerEl = ref<HTMLDivElement>()
let resizeObserver: ResizeObserver | undefined

onMounted(() => {
  if (!containerEl.value) return
  attachShell(props.spec, containerEl.value)
  resizeObserver = new ResizeObserver(() => fitShell(props.spec.id))
  resizeObserver.observe(containerEl.value)
})

onBeforeUnmount(() => resizeObserver?.disconnect())
</script>

<template>
  <div class="terminal-card fade-in">
    <div ref="containerEl" class="terminal-pane"></div>
    <div v-if="shellAccountGone[spec.id]" class="restore-failure">
      <span class="restore-message">
        The account {{ shellAccountGone[spec.id] }} was deleted, so this tab can't continue under
        it. Continuing on the primary account bills the primary and sends this conversation to it.
      </span>
      <AppButton class="close-tab" @click="closeFailedShell(spec.id)">Close tab</AppButton>
      <AppButton variant="primary" @click="continueOnPrimary(spec.id)">
        Continue on primary
      </AppButton>
    </div>
    <div v-else-if="shellRestoreFailures[spec.id]" class="restore-failure">
      <span class="restore-message">
        This tab could not resume its previous session. The agent's message is shown above.
      </span>
      <AppButton class="close-tab" @click="closeFailedShell(spec.id)">Close tab</AppButton>
      <AppButton variant="primary" @click="startFreshShell(spec.id)">Start a new session</AppButton>
    </div>
  </div>
</template>

<style scoped>
/* FitAddon sizes the terminal off the inner element's own box, without
   accounting for any padding or border on it, so the card chrome lives on
   this wrapper instead. */
.terminal-card {
  flex: 1;
  min-height: 0;
  width: 100%;
  padding: var(--space-3);
  background: var(--color-surface-1);
  border: 1px solid var(--color-border-subtle);
  border-radius: var(--radius-lg);
  overflow: hidden;
  position: relative;
}

.terminal-pane {
  height: 100%;
  width: 100%;
}

.restore-failure {
  position: absolute;
  left: var(--space-3);
  right: var(--space-3);
  bottom: var(--space-3);
  display: flex;
  align-items: center;
  gap: var(--space-3);
  padding: var(--space-3);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
}

.restore-message {
  flex: 1;
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
}

/* The failure bar sits on surface-2, so the ghost hover needs one step brighter to show. */
.restore-failure .close-tab:hover:not(:disabled) {
  background: var(--color-surface-3);
}
</style>
