// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { shellTitles } from './shellRegistry'
import { workgroupStateOf } from './workgroups'
import { shellTabById } from './workspace'

// How the app names a tab in a sentence. A tab title is free text a model may have set, so it is
// always marked, never run into the sentence as plain words; a workgroup member goes by its label and group.

export interface NamePart {
  text: string
  // Rendered highlighted by MarkedText.vue.
  mark?: boolean
}

const AGENT_NAMES: Record<string, string> = { claude: 'Claude', codex: 'Codex', agent: 'Cursor' }

// Agent CLIs prefix their terminal title with a status glyph, which is not part of the name.
export function cleanTitle(title: string): string {
  return title.replace(/^[^\p{L}\p{N}]+/u, '').trim()
}

function memberParts(projectId: string, tabId: string): NamePart[] | null {
  for (const group of workgroupStateOf(projectId).groups) {
    const member = group.members.find((m) => m.tabId === tabId)
    if (!member) continue
    return [
      { text: member.label, mark: true },
      { text: ' in workgroup ' },
      { text: group.name, mark: true }
    ]
  }
  return null
}

// Starts lower case except for agent names and marked labels, so it reads mid-sentence; MarkedText
// capitalizes plain words at the start of one.
// closedTitle is the title saved earlier, for naming a tab that has closed since.
export function tabParts(tabId: string, closedTitle?: string): NamePart[] {
  const found = shellTabById(tabId)
  if (!found) {
    const title = closedTitle ? cleanTitle(closedTitle) : ''
    if (!title) return [{ text: 'an agent in a closed tab' }]
    return [{ text: 'the closed tab ' }, { text: title, mark: true }]
  }
  const member = memberParts(found.project.id, tabId)
  if (member) return member
  const agent = AGENT_NAMES[found.tab.command] ?? 'an agent'
  const title = cleanTitle(shellTitles[tabId] ?? '')
  if (!title) return [{ text: `${agent} in an untitled tab` }]
  return [{ text: `${agent} in ` }, { text: title, mark: true }, { text: ' tab' }]
}

export function plainText(parts: NamePart[]): string {
  return parts.map((part) => part.text).join('')
}

// A marked first part is a name and keeps its own case.
export function capitalizeParts(parts: NamePart[]): NamePart[] {
  const [first, ...rest] = parts
  if (!first || first.mark) return parts
  return [{ ...first, text: first.text.charAt(0).toUpperCase() + first.text.slice(1) }, ...rest]
}
