// @ts-ignore
import type { Register } from 'claude-code'

// @ts-ignore
const DEFERRED = new Set(['Artifact', 'Workflow', 'ScheduleWakeup'])

export const register: Register = (on) => {
  on('tool.describe', async ($, e, next) => {
    const described = await next(e)
    return DEFERRED.has(e.tool) ? { ...described, isDeferred: true } : described
  })
}
