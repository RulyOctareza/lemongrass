// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// What a parent can call on an AppInput through a template ref.
export interface AppInputHandle {
  focus: () => void
  select: () => void
}
