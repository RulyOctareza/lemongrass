// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// Bundled, not fetched at runtime: xterm measures its cell once when a terminal opens, so a web
// font arriving later would leave terminals sized for the fallback face.
import '@fontsource-variable/comfortaa/wght.css'
import '@fontsource-variable/dm-sans/opsz.css'
import '@fontsource-variable/dm-sans/opsz-italic.css'
import '@fontsource-variable/jetbrains-mono/wght.css'
import '@fontsource-variable/jetbrains-mono/wght-italic.css'
import './style.css'
import './settings.css'
import './controls.css'

import { createApp } from 'vue'
import App from './App.vue'

createApp(App).mount('#app')
