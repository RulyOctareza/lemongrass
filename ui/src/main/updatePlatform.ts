// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// What differs between the rpm, the macOS and the Windows update. The check, download and SHA256SUMS
// verification are shared in updater.ts.
export interface UpdatePlatform {
  unsupportedReason(): Promise<string | null>
  assetName(version: string): string
  // Shown while install runs, null for the view's default.
  installingMessage: string | null
  // Installs the verified file and returns a message for the installed state, null for the default.
  // onInstalling is called once the install itself runs, after any password prompt.
  install(file: string, version: string, onInstalling: () => void): Promise<string | null>
  // The rpm is installed once install returns, so its download folder goes. The macOS app and the
  // Windows setup.exe are only staged there and are applied by restart.
  keepAfterInstall: boolean
  restart(): void
}
