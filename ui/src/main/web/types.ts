// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

export interface TokenPlacement {
  Kind: 'header' | 'cookie' | 'query'
  Name: string
  Prefix: string
}

export interface LoginStep {
  Action: 'goto' | 'fill' | 'click' | 'wait'
  URL: string
  Selector: string
  Field: string
  TimeoutSeconds: number
}

// What the vault's web_login_fields op returns for one user. Only the engine holds `fields`: nothing
// in a command result may carry a value from it.
export interface WebLoginFields {
  environment: string
  origin: string
  base_url: string
  login: LoginStep[] | null
  fields: Record<string, string> | null
  token_placement: TokenPlacement | null
  token_path: string
  user?: string
}

// The api side's hints that tell redaction where the rester's token travels.
export interface RedactHints {
  tokenPlacement: TokenPlacement | null
  tokenPath: string
}

export interface WebFlags {
  user?: string
  submit?: string
  text?: string
  gone?: string
  url?: string
  failed?: string
  errors?: string
  since?: string
  within?: string
}

export interface WebResult {
  text: string
  data?: string
  // The file name a download's server suggested.
  name?: string
}

// A command failure whose message is meant to be read by the model as is.
export class WebError extends Error {}
