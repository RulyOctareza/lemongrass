<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { ref, toRef } from 'vue'
import '../vault.css'
import { vaultSession, lockVaultSession, noteVaultActivity } from '../vaultSession'
import ConnectionsSection from './connector/ConnectionsSection.vue'
import DbChannelsSection from './connector/DbChannelsSection.vue'
import ResterList from './connector/rester/ResterList.vue'
import HttpChannelsSection from './connector/HttpChannelsSection.vue'
import AppButton from './AppButton.vue'
import VaultUnlockGate from './VaultUnlockGate.vue'
import DecisionsSection from './decisions/DecisionsSection.vue'
import { askState } from '../asks'

const props = defineProps<{
  active: boolean
}>()

const activeKind = ref<'decisions' | 'database' | 'http'>('decisions')
const activeDbTab = ref<'connections' | 'channels'>('connections')
const activeHttpTab = ref<'resters' | 'httpChannels'>('resters')

const vaultUnlocked = toRef(vaultSession, 'unlocked')

function shown(kind: 'database' | 'http', tab: string): boolean {
  if (!props.active || activeKind.value !== kind) return false
  return (kind === 'database' ? activeDbTab.value : activeHttpTab.value) === tab
}
</script>

<template>
  <div class="vault-tab" @click.capture="noteVaultActivity" @keydown.capture="noteVaultActivity">
    <div class="vault-frame" :class="{ 'vault-frame--gate': !vaultUnlocked }">
      <VaultUnlockGate v-if="!vaultUnlocked" />

      <template v-else>
        <nav class="vault-nav">
          <button
            class="nav-item"
            :class="{ active: activeKind === 'decisions' }"
            @click="activeKind = 'decisions'"
          >
            <svg
              class="nav-icon"
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <path d="M2.5 2.5h9v6.5h-5l-2.5 2.5v-2.5h-1.5z" />
              <path d="M5 5.5l1.4 1.4L9 4.3" />
            </svg>
            Decisions
            <span v-if="askState.count.waiting > 0" class="nav-count">
              {{ askState.count.waiting }}
            </span>
          </button>
          <span class="nav-group-label">Connector</span>
          <button
            class="nav-item"
            :class="{ active: activeKind === 'database' }"
            @click="activeKind = 'database'"
          >
            <svg
              class="nav-icon"
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <ellipse cx="7" cy="3.5" rx="4.5" ry="1.8" />
              <path d="M2.5 3.5v7c0 1 2 1.8 4.5 1.8s4.5-.8 4.5-1.8v-7" />
              <path d="M2.5 7c0 1 2 1.8 4.5 1.8S11.5 8 11.5 7" />
            </svg>
            Database
          </button>
          <button
            class="nav-item"
            :class="{ active: activeKind === 'http' }"
            @click="activeKind = 'http'"
          >
            <svg
              class="nav-icon"
              width="14"
              height="14"
              viewBox="0 0 14 14"
              fill="none"
              stroke="currentColor"
              stroke-width="1.2"
              stroke-linecap="round"
              stroke-linejoin="round"
            >
              <circle cx="7" cy="7" r="5.5" />
              <path d="M1.5 7h11" />
              <path
                d="M7 1.5c1.7 1.6 2.5 3.4 2.5 5.5S8.7 10.9 7 12.5C5.3 10.9 4.5 9.1 4.5 7S5.3 3.1 7 1.5z"
              />
            </svg>
            Rester
          </button>
          <div class="nav-lock">
            <span class="unlock-status">Vault unlocked</span>
            <AppButton @click="lockVaultSession">Lock</AppButton>
          </div>
        </nav>

        <div class="vault-main">
          <div class="vault-content">
            <div v-show="activeKind === 'decisions'" class="kind-pane">
              <DecisionsSection :visible="active && activeKind === 'decisions'" />
            </div>

            <div v-show="activeKind === 'database'" class="kind-pane">
              <div class="tab-row">
                <button
                  class="tab"
                  :class="{ active: activeDbTab === 'connections' }"
                  @click="activeDbTab = 'connections'"
                >
                  Connections
                </button>
                <button
                  class="tab"
                  :class="{ active: activeDbTab === 'channels' }"
                  @click="activeDbTab = 'channels'"
                >
                  Channels
                </button>
              </div>
              <ConnectionsSection
                v-show="activeDbTab === 'connections'"
                :visible="shown('database', 'connections')"
              />
              <DbChannelsSection
                v-show="activeDbTab === 'channels'"
                :visible="shown('database', 'channels')"
              />
            </div>

            <div v-show="activeKind === 'http'" class="kind-pane">
              <div class="tab-row">
                <button
                  class="tab"
                  :class="{ active: activeHttpTab === 'resters' }"
                  @click="activeHttpTab = 'resters'"
                >
                  Resters
                </button>
                <button
                  class="tab"
                  :class="{ active: activeHttpTab === 'httpChannels' }"
                  @click="activeHttpTab = 'httpChannels'"
                >
                  Channels
                </button>
              </div>
              <ResterList
                v-show="activeHttpTab === 'resters'"
                :visible="shown('http', 'resters')"
              />
              <HttpChannelsSection
                v-show="activeHttpTab === 'httpChannels'"
                :visible="shown('http', 'httpChannels')"
              />
            </div>
          </div>
        </div>
      </template>
    </div>
  </div>
</template>
