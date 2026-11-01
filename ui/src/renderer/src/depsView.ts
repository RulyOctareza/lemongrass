// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { DepEntry, DepOffer } from '../../preload/types'

// lgrassd reports features as fixed ids; an id with no copy here is shown as it is.
const FEATURES: Record<string, string> = {
  gitPane: 'the Git pane',
  conflicts: 'conflict resolution',
  rebaseForecast: 'the rebase forecast',
  biblioHistory: 'biblio history',
  workspaces: 'workspaces',
  device: 'the Device pane',
  flutterRecording: 'Flutter test recording',
  sshChannels: 'database channels over SSH'
}

export function listText(items: string[]): string {
  if (items.length <= 1) return items[0] ?? ''
  return `${items.slice(0, -1).join(', ')} and ${items[items.length - 1]}`
}

export function featuresText(ids: string[]): string {
  if (ids.length === 0) return ''
  return `Used by ${listText(ids.map((id) => FEATURES[id] ?? id))}.`
}

export function statusText(dep: DepEntry): string {
  switch (dep.status) {
    case 'ok':
      return dep.version || 'Installed'
    case 'missing':
      return 'Not installed'
    case 'belowFloor':
      return `${dep.version} is older than ${dep.floor}`
    case 'unknownVersion':
      return 'Version unknown'
  }
}

export function originText(dep: DepEntry): string {
  switch (dep.origin.kind) {
    case 'package':
      return `from ${dep.origin.package}`
    case 'manual':
      return 'installed by hand'
    default:
      return ''
  }
}

// A required dependency that is not usable gets the primary button, an optional one a ghost button,
// and one with nothing to offer no button at all.
export function buttonWeight(dep: DepEntry): 'primary' | 'ghost' | null {
  if (!dep.offer) return null
  return dep.required && dep.status !== 'ok' ? 'primary' : 'ghost'
}

export function actionLabel(offer: DepOffer): string {
  return offer.action === 'upgrade' ? 'Update' : 'Install'
}

// Only the Linux helper route runs from the app so far.
export function routeBlocked(offer: DepOffer): string | null {
  return offer.route === 'helper'
    ? null
    : 'Installing this from lemongrass is not supported on this system yet.'
}

export function tone(dep: DepEntry): 'ok' | 'warn' | 'muted' {
  if (dep.status === 'ok') return 'ok'
  return dep.required ? 'warn' : 'muted'
}

export function osText(os: { family: string; id: string; version: string }): string {
  const name = os.id ? os.id.charAt(0).toUpperCase() + os.id.slice(1) : 'Unknown system'
  return os.version ? `${name} ${os.version}` : name
}
