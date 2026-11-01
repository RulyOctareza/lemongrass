<!-- SPDX-License-Identifier: AGPL-3.0-only -->
<!-- Copyright (C) 2026 The Lemongrass Authors -->

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref } from 'vue'
import ProjectSidebar from './components/ProjectSidebar.vue'
import ProjectSwitcher from './components/ProjectSwitcher.vue'
import HeaderBar from './components/HeaderBar.vue'
import WorkspaceView from './components/WorkspaceView.vue'
import SettingsModal from './components/SettingsModal.vue'
import GuideModal from './components/GuideModal.vue'
import WorkgroupApprovalModal from './components/WorkgroupApprovalModal.vue'
import WorkgroupJoinModal from './components/WorkgroupJoinModal.vue'
import ProjectTrustModal from './components/ProjectTrustModal.vue'
import ZoomIndicator from './components/ZoomIndicator.vue'
import ProjectSelector from './components/ProjectSelector.vue'
import { loadAccounts } from './accounts'
import { startContextFeed, startUsagePolling } from './usage'
import { restoreZoom, setZoom, stepZoom } from './zoom'
import {
  cancelShellPicker,
  ensureLoaded,
  openVault,
  refreshReadOnlyDocs,
  shellPicker,
  type ProjectRef
} from './workspace'
import type { Project, BiblioTree } from '../../preload/types'

const projects = ref<Project[]>([])
const activeProjectId = ref<string | null>(null)

// Fetched on first visit and kept live by onBiblioChanged, including a biblio/ created after that visit.
const biblioByProject = reactive<Record<string, BiblioTree | null>>({})
const loadedProjects = reactive<Record<string, boolean>>({})
const sidebarCollapsed = ref(false)
const closing = ref(false)
const settingsOpen = ref(false)
const guideOpen = ref(false)
const trustPrompt = ref<Project | null>(null)
const trustBusy = ref(false)

const activeProject = computed((): Project | undefined =>
  projects.value.find((p) => p.id === activeProjectId.value)
)
const activeRef = computed(
  (): ProjectRef | undefined =>
    activeProject.value && { id: activeProject.value.id, path: activeProject.value.path }
)
const biblioTree = computed((): BiblioTree | null =>
  activeProjectId.value ? (biblioByProject[activeProjectId.value] ?? null) : null
)

// With the project selector chosen for start-up, nothing is selected, so no workspace loads and no
// shell spawns until the human picks a project.
async function restoreUiState(): Promise<void> {
  const saved = await window.api.uiState.load()
  void restoreZoom(saved.zoomFactor)
  projects.value = await window.api.projects.list()
  if (saved.startup === 'projectSelector' || projects.value.length === 0) return
  const last = projects.value.find((p) => p.id === saved.lastProjectId)
  await selectProject((last ?? projects.value[0]).id)
}

async function selectProject(id: string): Promise<void> {
  // shellPicker.placement pins the project it was opened for, so a project switch while it is open must cancel it.
  if (shellPicker.open) cancelShellPicker()
  activeProjectId.value = id
  window.api.uiState.save({ lastProjectId: id })
  const project = projects.value.find((p) => p.id === id)
  if (project && project.trusted === undefined) trustPrompt.value = project
  if (!project || loadedProjects[id]) return
  await ensureLoaded({ id, path: project.path })
  biblioByProject[id] = await window.api.biblio.tree(project.path)
  loadedProjects[id] = true
}

async function refreshBiblio(): Promise<void> {
  const id = activeProjectId.value
  if (!id || !activeProject.value) return
  biblioByProject[id] = await window.api.biblio.tree(activeProject.value.path)
}

// Fires whenever anything adds/removes a file under a project's biblio/ --
// not only the app's own createScratchpad IPC call.
async function onBiblioChanged(projectPath: string): Promise<void> {
  const project = projects.value.find((p) => p.path === projectPath)
  if (!project) return
  biblioByProject[project.id] = await window.api.biblio.tree(projectPath)
  void refreshReadOnlyDocs({ id: project.id, path: project.path })
}

async function answerTrust(trusted: boolean): Promise<void> {
  const project = trustPrompt.value
  if (!project) return
  trustBusy.value = true
  try {
    const updated = await window.api.projects.setTrust(project.id, trusted)
    if (updated) {
      projects.value = projects.value.map((p) => (p.id === updated.id ? updated : p))
    }
  } finally {
    trustBusy.value = false
    trustPrompt.value = null
  }
}

async function addProject(): Promise<void> {
  const project = await window.api.projects.add()
  if (!project) return
  if (!projects.value.find((p) => p.id === project.id)) projects.value.push(project)
  await selectProject(project.id)
}

// Captured, not bubbled -- xterm treats some Ctrl+key combos (Ctrl+- among
// them) as shell input and cancels the browser event on its own element
// before it would ever bubble up to a window-level listener.
function onKeydown(event: KeyboardEvent): void {
  if (!(event.ctrlKey || event.metaKey)) return
  if (event.key === '=' || event.key === '+') {
    event.preventDefault()
    event.stopPropagation()
    void stepZoom(1)
  } else if (event.key === '-') {
    event.preventDefault()
    event.stopPropagation()
    void stepZoom(-1)
  } else if (event.key === '0') {
    event.preventDefault()
    event.stopPropagation()
    setZoom(1)
  }
}

let unsubscribeBiblio: (() => void) | undefined
let unsubscribeClosing: (() => void) | undefined

onMounted(() => {
  void restoreUiState()
  void loadAccounts()
  startUsagePolling()
  startContextFeed()
  window.addEventListener('keydown', onKeydown, { capture: true })
  unsubscribeBiblio = window.api.biblio.onChanged(onBiblioChanged)
  unsubscribeClosing = window.api.windowControls.onClosing(() => (closing.value = true))
})
onBeforeUnmount(() => {
  window.removeEventListener('keydown', onKeydown, { capture: true })
  unsubscribeBiblio?.()
  unsubscribeClosing?.()
})
</script>

<template>
  <div class="shell">
    <HeaderBar
      :sidebar-collapsed="sidebarCollapsed"
      @toggle-sidebar="sidebarCollapsed = !sidebarCollapsed"
      @open-settings="settingsOpen = true"
      @open-guide="guideOpen = true"
    >
      <template #title>
        <span class="brand-mark">
          <svg
            width="12"
            height="12"
            viewBox="0 0 24 24"
            fill="none"
            stroke="var(--color-black)"
            stroke-width="2.4"
            stroke-linecap="round"
            stroke-linejoin="round"
          >
            <path d="M11 20A7 7 0 0 1 9.8 6.1C15.5 5 17 4.48 19.2 2.2c1.7 6.6.2 13.1-5 17.7" />
            <path d="M2 21c0-3 1.85-5.36 5.08-6" />
          </svg>
        </span>
        <span class="wordmark">lemongrass</span>
        <span class="switcher-zone">
          <ProjectSwitcher
            :projects="projects"
            :active-project-id="activeProjectId"
            @select="selectProject"
            @add="addProject"
          />
        </span>
      </template>
    </HeaderBar>

    <div class="body-row">
      <ProjectSidebar
        :collapsed="sidebarCollapsed"
        :project="activeRef"
        :tree="biblioTree"
        @open-vault="activeRef && openVault(activeRef)"
        @refresh-biblio="refreshBiblio"
      />

      <div v-if="activeRef && loadedProjects[activeRef.id]" class="main">
        <WorkspaceView :project="activeRef" />
      </div>

      <div v-else-if="!activeProject" class="main">
        <ProjectSelector :projects="projects" @select="selectProject" @add="addProject" />
      </div>
    </div>

    <SettingsModal
      v-if="settingsOpen"
      :projects="projects"
      :active-project-id="activeProjectId"
      @close="settingsOpen = false"
    />
    <GuideModal v-if="guideOpen" @close="guideOpen = false" />
    <WorkgroupApprovalModal />
    <WorkgroupJoinModal />
    <ZoomIndicator />
    <ProjectTrustModal
      v-if="trustPrompt"
      :project="trustPrompt"
      :busy="trustBusy"
      @answer="answerTrust"
      @close="trustPrompt = null"
    />

    <div v-if="closing" class="closing-overlay">
      <div class="closing-card">
        <span class="closing-spinner"></span>
        <div class="closing-text">
          <p class="closing-title">Closing agent sessions</p>
          <p class="closing-detail">
            Saving each Codex session so it can resume on the next launch.
          </p>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.closing-overlay {
  position: fixed;
  inset: 0;
  z-index: 1000;
  display: flex;
  align-items: center;
  justify-content: center;
  background: color-mix(in srgb, var(--color-surface-0) 80%, transparent);
}

.closing-card {
  display: flex;
  align-items: center;
  gap: var(--space-4);
  padding: var(--space-5) var(--space-6);
  background: var(--color-surface-2);
  border: 1px solid var(--color-border-default);
  border-radius: var(--radius-lg);
}

.closing-spinner {
  width: 16px;
  height: 16px;
  border: 2px solid var(--color-border-default);
  border-top-color: var(--color-amber);
  border-radius: 50%;
  animation: closing-spin 0.8s linear infinite;
}

.closing-title {
  color: var(--color-fg-primary);
  font-family: var(--font-body);
  font-size: var(--text-sm);
  font-weight: var(--weight-medium);
}

.closing-detail {
  margin-top: var(--space-1);
  color: var(--color-fg-secondary);
  font-family: var(--font-body);
  font-size: var(--text-xs);
}

@keyframes closing-spin {
  to {
    transform: rotate(360deg);
  }
}

.shell {
  height: 100%;
  display: flex;
  flex-direction: column;
  background: var(--color-surface-0);
}

.brand-mark {
  display: flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: var(--radius-sm);
  background: var(--color-amber);
  flex-shrink: 0;
  margin-right: var(--space-2);
}

.wordmark {
  font-family: var(--font-display);
  font-size: var(--text-md);
  font-weight: var(--weight-bold);
  color: var(--color-fg-accent);
  letter-spacing: var(--tracking-snug);
}

.switcher-zone {
  margin-left: var(--space-3);
  -webkit-app-region: no-drag;
}

.body-row {
  flex: 1;
  min-height: 0;
  display: flex;
}

.main {
  flex: 1;
  min-width: 0;
  min-height: 0;
  display: flex;
  gap: var(--space-2);
}
</style>
