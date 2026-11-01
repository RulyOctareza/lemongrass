// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { Ask, Project } from '../../../../preload/types'
import { shellTitles } from '../../shellRegistry'
import { shellTabById } from '../../workspace'

// Who asked and where, in words a person reads without the ask id: the agent and its tab title, and
// the project the ask's folder belongs to.

const projects = reactive<{ list: Project[] }>({ list: [] })

export function loadAskProjects(): void {
  window.api.projects
    .list()
    .then((list) => (projects.list = list))
    .catch(() => undefined)
}

const AGENT_NAMES: Record<string, string> = { claude: 'Claude', codex: 'Codex', agent: 'Cursor' }

export interface AskOrigin {
  // "Claude in Fix login", or "an agent in a closed tab" once the tab is gone.
  who: string
  // "lemongrass", or "the ui folder of lemongrass" below the project root.
  where: string
}

// Agent CLIs prefix their terminal title with a status glyph, which is not part of the name.
function cleanTitle(title: string): string {
  return title.replace(/^[^\p{L}\p{N}]+/u, '').trim()
}

function whoAsked(tabId: string): string {
  const found = shellTabById(tabId)
  if (!found) return 'an agent in a closed tab'
  const agent = AGENT_NAMES[found.tab.command] ?? 'an agent'
  const title = cleanTitle(shellTitles[tabId] ?? '')
  return title ? `${agent} in ${title}` : agent
}

function whereAsked(cwd: string): string {
  const owner = projects.list
    .filter((p) => cwd === p.path || cwd.startsWith(p.path.replace(/\/$/, '') + '/'))
    .sort((a, b) => b.path.length - a.path.length)[0]
  if (!owner) return cwd
  const rest = cwd.slice(owner.path.replace(/\/$/, '').length).replace(/^\//, '')
  return rest ? `the ${rest} folder of ${owner.name}` : owner.name
}

export function capitalize(text: string): string {
  return text.charAt(0).toUpperCase() + text.slice(1)
}

export function askOrigin(ask: Ask): AskOrigin {
  return { who: whoAsked(ask.tab_id), where: whereAsked(ask.cwd) }
}

function listWords(items: string[]): string {
  if (items.length <= 1) return items[0] ?? ''
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

// The one sentence an ask opens with: who wants what, and where.
export function askSentence(ask: Ask): string {
  const { where } = askOrigin(ask)
  const who = capitalize(askOrigin(ask).who)
  if (ask.type === 'db') {
    const tables = ask.tables ?? []
    const what = tables.length
      ? `a database channel for ${listWords(tables)}`
      : 'a database channel'
    return `${who} wants ${what} in ${where}.`
  }
  if (ask.type === 'rester') return `${who} wants a rester channel in ${where}.`
  return `${who} wants to run a command in ${where}.`
}
