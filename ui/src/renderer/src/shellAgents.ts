// SPDX-License-Identifier: AGPL-3.0-only
// Copyright (C) 2026 The Lemongrass Authors

export interface ShellAgent {
  id: string
  command: string
  label: string
  hint: string
  warning?: string
}

export const SHELL_AGENTS: readonly ShellAgent[] = [
  {
    id: 'claude',
    command: 'claude',
    label: 'Claude Code',
    hint: 'claude'
  },
  {
    id: 'codex',
    command: 'codex',
    label: 'Codex',
    hint: 'codex'
  },
  {
    id: 'cursor',
    command: 'agent',
    label: 'Cursor Agent',
    hint: 'agent',
    warning: 'Cursor support is still under development. Use Claude Code or Codex.'
  },
  {
    id: 'shell',
    command: 'zsh',
    label: 'Shell',
    hint: 'shell'
  }
]

const RESUME_ARGV: Record<string, (sessionId: string) => string[]> = {
  claude: (sessionId) => ['--resume', sessionId],
  codex: (sessionId) => ['resume', sessionId]
}

// The lgrass PreToolUse hook is the permission gate for these agents, so their own prompts are switched off.
const BYPASS_ARGV: Record<string, string[]> = {
  claude: ['--dangerously-skip-permissions']
}

export function spawnArgs(command: string, tabId: string, args?: string[]): string[] | undefined {
  const marker =
    command === 'codex'
      ? ['--no-daemon', '-c', `shell_environment_policy.set.LGRASS_TAB_ID=${JSON.stringify(tabId)}`]
      : []
  const result = [...marker, ...(BYPASS_ARGV[command] ?? []), ...(args ?? [])]
  return result.length > 0 ? result : undefined
}

export function isResumableCommand(command: string): boolean {
  return command in RESUME_ARGV
}

export function resumeArgs(command: string, sessionId: string): string[] | null {
  return RESUME_ARGV[command]?.(sessionId) ?? null
}

const PREF_KEY = 'lemongrass.shellAgentId'

export function preferredShellAgentId(): string {
  try {
    const saved = localStorage.getItem(PREF_KEY)
    if (saved && SHELL_AGENTS.some((agent) => agent.id === saved)) return saved
  } catch {
    // ignore
  }
  return 'claude'
}

export function setPreferredShellAgentId(id: string): void {
  try {
    localStorage.setItem(PREF_KEY, id)
  } catch {
    // ignore
  }
}

export function shellAgentById(id: string): ShellAgent {
  return SHELL_AGENTS.find((agent) => agent.id === id) ?? SHELL_AGENTS[0]
}
