import { existsSync } from 'node:fs'
import { join } from 'node:path'

// Built-in features no lemongrass tab uses, removed so every Claude session starts with less context.
const TRIM_ENV: Record<string, string> = {
  CLAUDE_CODE_DISABLE_BUNDLED_SKILLS: '1',
  ENABLE_CLAUDEAI_MCP_SERVERS: 'false'
}

// EnterPlanMode is banned by the lgrass guard, so its exit tool has no use either.
const TRIM_TOOLS = [
  'SendFeedback',
  'ShareOnboardingGuide',
  'ReportFindings',
  'EnterPlanMode',
  'ExitPlanMode'
]

export function startupTrimEnv(command: string): Record<string, string> {
  return command === 'claude' ? { ...TRIM_ENV } : {}
}

// lgrassconf installs the plugin that moves rarely used tools behind ToolSearch.
export function startupTrimArgs(command: string, home: string): string[] {
  if (command !== 'claude') return []
  const plugin = join(home, '.lemongrass', 'plugin', 'lemongrass-tools')
  return [
    '--disallowedTools',
    TRIM_TOOLS.join(' '),
    ...(existsSync(plugin) ? ['--plugin-dir', plugin] : [])
  ]
}
