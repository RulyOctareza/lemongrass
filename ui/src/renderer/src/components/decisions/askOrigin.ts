// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import { reactive } from 'vue'
import type { Ask, Project } from '../../../../preload/types'
import { capitalizeParts, plainText, tabParts, type NamePart } from '../../tabNames'

// Who asked and where, in words a person reads without the ask id: the tab by its quoted title or its
// workgroup role, and the project the ask's folder belongs to.

const projects = reactive<{ list: Project[] }>({ list: [] })

export function loadAskProjects(): void {
  window.api.projects
    .list()
    .then((list) => (projects.list = list))
    .catch(() => undefined)
}

// "lemongrass", or "the ui folder of lemongrass" below the project root.
export function askWhere(ask: Ask): string {
  const cwd = ask.cwd
  const owner = projects.list
    .filter((p) => cwd === p.path || cwd.startsWith(p.path.replace(/\/$/, '') + '/'))
    .sort((a, b) => b.path.length - a.path.length)[0]
  if (!owner) return cwd
  const rest = cwd.slice(owner.path.replace(/\/$/, '').length).replace(/^\//, '')
  return rest ? `the ${rest} folder of ${owner.name}` : owner.name
}

function wants(ask: Ask): string {
  // The tables show as capsules in the grant view, so the sentence stays short however many there are.
  if (ask.type === 'db') return ' wants a database channel'
  if (ask.type === 'rester') return ' wants a rester channel'
  return ' wants to run a command'
}

// The headline an ask opens with, such as: Claude in "Fix login" tab wants to run a command.
export function askParts(ask: Ask): NamePart[] {
  return capitalizeParts([...tabParts(ask.tab_id), { text: `${wants(ask)}.` }])
}

// The same sentence as plain text with the project, for lists that span projects.
export function askSentence(ask: Ask): string {
  const parts = capitalizeParts([...tabParts(ask.tab_id), { text: wants(ask) }])
  return `${plainText(parts)} in ${askWhere(ask)}.`
}
