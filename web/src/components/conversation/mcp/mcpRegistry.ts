import type { AgentEvent } from '../../../lib/types'

export interface McpServerDescriptor {
  readonly id: string
  matchesEvent(event: AgentEvent): boolean
  resolveToolOp(baseTool: string): string | null
}

export function resolveMcpServer(_event: AgentEvent): McpServerDescriptor | null {
  return null
}

export function stripMcpNamespace(raw: string): string {
  let name = raw.trim()

  if (name.includes(':')) {
    name = name.slice(name.indexOf(':') + 1).trim() || name
  }

  if (name.includes('__')) {
    const parts = name.split('__')
    name = parts[parts.length - 1] || name
  }

  if (name.includes('/')) {
    name = name.slice(name.lastIndexOf('/') + 1) || name
  }

  return name.trim()
}
