// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Release versions are the tag's semver without the leading v. Build metadata after + is ignored.
interface Parsed {
  core: number[]
  pre: string[]
}

function parse(version: string): Parsed | null {
  const match = /^v?(\d+)\.(\d+)\.(\d+)(?:-([0-9A-Za-z.-]+))?(?:\+.*)?$/.exec(version.trim())
  if (!match) return null
  return {
    core: [Number(match[1]), Number(match[2]), Number(match[3])],
    pre: match[4] ? match[4].split('.') : []
  }
}

function comparePre(a: string[], b: string[]): number {
  // A version without a prerelease part sorts after every prerelease of it, so an installed release
  // candidate is offered the final release.
  if (a.length === 0 || b.length === 0) return Math.sign(b.length - a.length)
  for (let i = 0; i < Math.min(a.length, b.length); i++) {
    const an = /^\d+$/.test(a[i])
    const bn = /^\d+$/.test(b[i])
    if (an && bn) {
      const diff = Number(a[i]) - Number(b[i])
      if (diff !== 0) return Math.sign(diff)
    } else if (an !== bn) {
      return an ? -1 : 1
    } else if (a[i] !== b[i]) {
      return a[i] < b[i] ? -1 : 1
    }
  }
  return Math.sign(a.length - b.length)
}

// Returns null when either side is not semver.
export function compareVersions(a: string, b: string): number | null {
  const pa = parse(a)
  const pb = parse(b)
  if (!pa || !pb) return null
  for (let i = 0; i < 3; i++) {
    if (pa.core[i] !== pb.core[i]) return Math.sign(pa.core[i] - pb.core[i])
  }
  return comparePre(pa.pre, pb.pre)
}

export function isNewer(latest: string, current: string): boolean {
  return (compareVersions(latest, current) ?? 0) > 0
}

export function versionFromTag(tag: string): string {
  return tag.replace(/^v/, '')
}

// Must match rpm.artifactName in electron-builder.yml.
export function rpmAssetName(version: string): string {
  return `lemongrass-${version}.x86_64.rpm`
}

// Must match dmg.artifactName in electron-builder.yml. arch is process.arch, arm64 or x64.
export function dmgAssetName(version: string, arch: string): string {
  return `lemongrass-${version}-${arch}.dmg`
}

// Must match nsis.artifactName in electron-builder.yml.
export function winAssetName(version: string): string {
  return `lemongrass-${version}-setup.exe`
}

export interface ListedRelease {
  tag_name?: string
  draft?: boolean
}

// The prerelease channel reads the releases list, which GitHub orders by creation date, not version, so
// the newest is picked by semver. A tag that is not semver is skipped.
export function newestRelease<T extends ListedRelease>(releases: T[]): T | null {
  let best: T | null = null
  for (const rel of releases) {
    if (rel.draft || !rel.tag_name) continue
    const version = versionFromTag(rel.tag_name)
    if (compareVersions(version, version) === null) continue
    if (!best || isNewer(version, versionFromTag(best.tag_name ?? ''))) best = rel
  }
  return best
}

// Reads one file's hash from sha256sum output, where each line is the hex digest, two spaces (or a
// space and * in binary mode) and the file name.
export function sha256For(sums: string, file: string): string | null {
  for (const line of sums.split('\n')) {
    const match = /^([0-9a-fA-F]{64}) [ *](.+)$/.exec(line.trim())
    if (match && match[2] === file) return match[1].toLowerCase()
  }
  return null
}
