<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import IconFrame from './IconFrame.vue'
import { CLAUDE_MARK, CLAUDE_MARK_BOX, CODEX_MARK, CODEX_MARK_BOX } from '../vendorMarks'

// The vendor marks fill with currentColor here, unlike Settings, so a pad's icon follows its text
// from muted to white.
const MARKS: Record<string, { d: string; box: number }> = {
  claude: { d: CLAUDE_MARK, box: CLAUDE_MARK_BOX },
  codex: { d: CODEX_MARK, box: CODEX_MARK_BOX }
}

const SHELL_PROMPT = ['M1.5 2l5 5l-5 5', 'M7.5 12h5']

withDefaults(defineProps<{ agentId: string; size?: number }>(), { size: 14 })
</script>

<template>
  <IconFrame
    v-if="MARKS[agentId]"
    :size="size"
    :box="MARKS[agentId].box"
    :stroke-width="null"
    fill="currentColor"
  >
    <path :d="MARKS[agentId].d" />
  </IconFrame>
  <IconFrame v-else :size="size">
    <path v-for="d in SHELL_PROMPT" :key="d" :d="d" />
  </IconFrame>
</template>
