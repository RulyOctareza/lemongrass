// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// The slice of webContents.debugger the engine uses, so capture and login logic can be tested with a fake.
export interface Cdp {
  send<T = unknown>(method: string, params?: object): Promise<T>
}

export type CdpEventHandler = (method: string, params: Record<string, unknown>) => void
