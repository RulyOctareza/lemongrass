// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 Faizal Alviansyah

import type {
  VaultEnvironment,
  VaultHeader,
  VaultLoginFieldType,
  VaultRester,
  VaultResterAPI,
  VaultResterUser
} from '../../../../../preload/types'

export type FieldType = 'text' | VaultLoginFieldType

export interface FieldDraft {
  key: string
  value: string
  type: FieldType
}

export interface HeaderDraft {
  name: string
  value: string
}

export function defaultResterAPI(): VaultResterAPI {
  return {
    LoginEndpoint: '',
    TokenPath: '',
    TTLOrigin: '',
    FixedTTLSeconds: 3600,
    TokenPlacement: { Kind: 'header', Name: 'Authorization', Prefix: 'Bearer ' },
    Headers: null
  }
}

// IPC structured clone cannot clone a Vue reactive Proxy, so anything sent to the vault goes through here.
export function plainCopy<T>(value: T): T {
  return JSON.parse(JSON.stringify(value)) as T
}

export function headerDrafts(headers: VaultHeader[] | null): HeaderDraft[] {
  return (headers ?? []).map((h) => ({ name: h.Name, value: h.Value }))
}

export function headersFromDrafts(drafts: HeaderDraft[]): VaultHeader[] | null {
  const headers = drafts
    .filter((h) => h.name.trim() !== '')
    .map((h) => ({ Name: h.name.trim(), Value: h.value }))
  return headers.length > 0 ? headers : null
}

export function fieldDrafts(
  fields: Record<string, string> | null,
  types: Record<string, string> | null
): FieldDraft[] {
  return Object.entries(fields ?? {}).map(([key, value]) => ({
    key,
    value,
    type: (types?.[key] as FieldType | undefined) ?? 'text'
  }))
}

export function fieldsFromDrafts(drafts: FieldDraft[]): {
  Fields: Record<string, string>
  FieldTypes: Record<string, string> | null
} {
  const fields: Record<string, string> = {}
  const types: Record<string, string> = {}
  for (const f of drafts) {
    const key = f.key.trim()
    if (!key) continue
    fields[key] = f.value
    if (f.type !== 'text') types[key] = f.type
  }
  return { Fields: fields, FieldTypes: Object.keys(types).length > 0 ? types : null }
}

export function tagList(text: string): string[] {
  return text
    .split(',')
    .map((t) => t.trim())
    .filter((t) => t.length > 0)
}

// Mirrors the vault's effective user: override fields replace key by key, an override token replaces the token.
export function effectiveUser(user: VaultResterUser, environment: string): VaultResterUser {
  const override = user.Overrides?.[environment]
  const fields = { ...(user.Fields ?? {}), ...(override?.Fields ?? {}) }
  const types = { ...(user.FieldTypes ?? {}), ...(override?.FieldTypes ?? {}) }
  return {
    Name: user.Name,
    Fields: fields,
    FieldTypes: Object.keys(types).length > 0 ? types : null,
    Token: override?.Token ? override.Token : user.Token,
    Tags: user.Tags,
    Environments: null,
    Overrides: null
  }
}

export function fieldKeys(rester: VaultRester): string[] {
  const keys = new Set<string>()
  for (const user of rester.Users) {
    for (const key of Object.keys(user.Fields ?? {})) keys.add(key)
  }
  return [...keys].sort()
}

export function findEnvironment(rester: VaultRester, name: string): VaultEnvironment | undefined {
  return rester.Environments.find((e) => e.Name === name)
}

// Applies an edit to a freshly read copy of the rester and writes the whole record back; resolves to an error message, empty on success.
export type CommitRester = (mutate: (rester: VaultRester) => void) => Promise<string>
