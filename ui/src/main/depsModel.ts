// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

import type { DepEntry, DepOffer, DepOrigin, DepOutcome, DepsReport } from '../preload/types'

// Like gitMove.ts, this file has no value imports so node --test can load it straight from source.

const STATUSES = ['ok', 'missing', 'belowFloor', 'unknownVersion']
const ORIGINS = ['package', 'manual', 'none', 'unknown']
const ROUTES = ['helper', 'winget', 'brew']
export const KEPT_LINES = 6

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value)
}

function text(value: unknown): string {
  return typeof value === 'string' ? value : ''
}

function parseOrigin(value: unknown): DepOrigin | null {
  if (!isRecord(value) || !ORIGINS.includes(text(value.kind))) return null
  if (value.kind !== 'package') return { kind: value.kind as 'manual' | 'none' | 'unknown' }
  const name = text(value.package)
  return name ? { kind: 'package', package: name } : null
}

function parseOffer(value: unknown): DepOffer | null | undefined {
  if (value === null || value === undefined) return null
  if (!isRecord(value)) return undefined
  const action = text(value.action)
  const route = text(value.route)
  const name = text(value.package)
  if ((action !== 'install' && action !== 'upgrade') || !ROUTES.includes(route) || !name) {
    return undefined
  }
  return { action, package: name, route: route as DepOffer['route'] }
}

function parseEntry(value: unknown): DepEntry | null {
  if (!isRecord(value)) return null
  const name = text(value.name)
  const origin = parseOrigin(value.origin)
  const offer = parseOffer(value.offer)
  const status = text(value.status)
  if (!name || !origin || offer === undefined || !STATUSES.includes(status)) return null
  return {
    name,
    required: value.required === true,
    features: Array.isArray(value.features)
      ? value.features.filter((f) => typeof f === 'string')
      : [],
    floor: text(value.floor),
    path: text(value.path),
    version: text(value.version),
    origin,
    status: status as DepEntry['status'],
    offer,
    message: text(value.message)
  }
}

// The output of `lgrassd deps check --json`. A report the app cannot read in full is refused as a
// whole, since an install button built from a half-read entry could name the wrong package.
export function parseReport(stdout: string): DepsReport | null {
  let value: unknown
  try {
    value = JSON.parse(stdout)
  } catch {
    return null
  }
  if (!isRecord(value) || !isRecord(value.os) || !Array.isArray(value.deps)) return null
  const deps = value.deps.map(parseEntry)
  if (deps.some((dep) => dep === null)) return null
  return {
    os: { family: text(value.os.family), id: text(value.os.id), version: text(value.os.version) },
    deps: deps as DepEntry[]
  }
}

// The Go arch name of the helper's resource folder; the polkit policy names an action for each.
export function helperArch(arch: string): string | null {
  return arch === 'x64' ? 'amd64' : arch === 'arm64' ? 'arm64' : null
}

// Why this build cannot install anything at all, or null. The root helper ships only in the Linux rpm.
export function installUnsupported(
  packaged: boolean,
  platform: string,
  arch: string
): string | null {
  if (!packaged) return 'This is a source build. Installs run only in the installed app.'
  if (platform !== 'linux' || !helperArch(arch)) {
    return 'Installing from lemongrass is not supported on this system yet.'
  }
  return null
}

// Why one offer cannot run here, or null. Only the Linux helper route is built so far.
export function offerBlocked(offer: DepOffer | null, unsupported: string | null): string | null {
  if (!offer) return 'Lemongrass has nothing to install for this.'
  if (unsupported) return unsupported
  if (offer.route !== 'helper')
    return 'Installing from lemongrass is not supported on this system yet.'
  return null
}

// Adds a chunk of output to the kept lines. dnf redraws its progress bars with a carriage return, so
// that ends a line too, and only the newest lines are kept.
export function appendOutput(lines: string[], partial: string, chunk: string): [string[], string] {
  const parts = (partial + chunk).split(/\r\n|\r|\n/)
  const rest = parts.pop() ?? ''
  const added = parts.map((line) => line.trimEnd()).filter((line) => line !== '')
  return [[...lines, ...added].slice(-KEPT_LINES), rest]
}

// The sentence for how a `pkexec lgrassroot` run ended. The helper's codes are 1 (the package manager
// failed), 2 (refused, nothing ran) and 3 (another package manager holds the lock); pkexec's own are
// 126 (the prompt was dismissed) and 127 (not authorized). The helper's last stderr line starts with
// "lgrassroot: " and is shown when it has one.
export function helperExitText(code: number | null, lastError: string): string {
  const helperLine = lastError.startsWith('lgrassroot: ') ? lastError : ''
  switch (code) {
    case 126:
      return 'The password prompt was dismissed. Nothing was installed.'
    case 127:
      return 'Your account is not allowed to install packages. Nothing was installed.'
    case 2:
      return helperLine || 'Lemongrass refused this request. Nothing was installed.'
    case 3:
      return 'Another package manager is running. Try again once it finishes.'
    case 1:
      return helperLine || 'The package manager failed. Its last output is shown above.'
    case null:
      return 'The install was stopped before it finished.'
    default:
      return helperLine || `The install ended with code ${code}.`
  }
}

export type RequiredStatuses = Record<string, DepEntry['status']>

// Whether a required dependency that was ok when last seen is no longer ok, which reopens the startup layer.
export function requiredRegressed(seen: RequiredStatuses, report: DepsReport): boolean {
  return report.deps.some((dep) => dep.required && seen[dep.name] === 'ok' && dep.status !== 'ok')
}

// The statuses to keep after a check. A required dependency now ok is recorded at once; one that is
// not ok keeps its earlier record until the layer completes, so a layer closed without Continue
// still reopens on the next startup.
export function nextSeen(
  seen: RequiredStatuses,
  report: DepsReport,
  completed: boolean
): RequiredStatuses {
  const next: RequiredStatuses = {}
  for (const dep of report.deps) {
    if (!dep.required) continue
    next[dep.name] = completed || dep.status === 'ok' ? dep.status : (seen[dep.name] ?? dep.status)
  }
  return next
}

// The sentence for a helper run that exited 0. dnf also exits 0 with "Nothing to do" when the
// repositories have no newer package, so the check that follows decides whether it fixed anything.
export function successText(action: DepOffer['action'], after: DepEntry | undefined): DepOutcome {
  if (after && after.status !== 'ok') {
    return {
      ok: false,
      text:
        action === 'upgrade'
          ? `Your repositories have no newer ${after.name}.`
          : `${after.name} is still not usable after the install.`
    }
  }
  return { ok: true, text: action === 'upgrade' ? 'Updated.' : 'Installed.' }
}
