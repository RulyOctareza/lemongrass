// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

// @ts-ignore
import type { Register } from 'claude-code'

// @ts-ignore
const DEFERRED = new Set(['Artifact', 'Workflow', 'ScheduleWakeup'])

const ARTIFACT_LINE = /^Artifact: .*$/m
const ARTIFACT_HINT =
  'Artifact: publishes or reads claude.ai artifacts. Load it only when asked to publish, update or read one.'

export const register: Register = (on) => {
  on('tool.describe', async ($, e, next) => {
    const described = await next(e)
    return DEFERRED.has(e.tool) ? { ...described, isDeferred: true } : described
  })

  on('prompt.attachment', ($, e, next) =>
    e.type === 'deferred_tools_delta' && ARTIFACT_LINE.test(e.text)
      ? next({ ...e, text: e.text.replace(ARTIFACT_LINE, ARTIFACT_HINT) })
      : next(e)
  )
}
