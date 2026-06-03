import { useMemo } from 'react'
import { Activity, CheckCircle2, Clock, Search, Server, XCircle } from 'lucide-react'
import { useSpaceDetail } from '../../hooks/useSpaceDetail'
import { useSpaceRoster } from '../../hooks/useSpaceStatus'
import type { AgentEvent } from '../../lib/types'
import { cn } from '@/lib/utils'

interface SpaceInspectorTabProps {
  spaceId: string
}

interface McpToolCall {
  id: string
  toolName: string
  server: string
  member: string
  status: AgentEvent['status']
  startedAt: string
  completedAt?: string
  error?: string
}

function clean(value: unknown): string {
  return String(value ?? '').trim()
}

function stripMcpToolName(toolName: string): string {
  if (toolName.startsWith('mcp__')) {
    const parts = toolName.split('__').filter(Boolean)
    return parts.length >= 3 ? parts.slice(2).join('.') : parts.at(-1) ?? toolName
  }
  if (toolName.includes('/')) return toolName.split('/').slice(1).join('/') || toolName
  return toolName
}

function inferServer(event: AgentEvent, toolName: string): string {
  const explicit = clean(event.data?.mcpServerId ?? event.data?.server)
  if (explicit) return explicit
  if (toolName.startsWith('mcp__')) {
    const parts = toolName.split('__').filter(Boolean)
    if (parts.length >= 2) return parts[1]
  }
  if (toolName.includes('/')) return toolName.split('/')[0]
  const kind = clean(event.kind)
  if (kind.startsWith('mcp:')) return kind.slice(4)
  return 'mcp'
}

function isMcpToolEvent(event: AgentEvent): boolean {
  const kind = clean(event.kind).toLowerCase()
  const toolName = clean(event.data?.toolName ?? event.data?.toolNameRaw ?? event.title)
  const sourceType = clean(event.data?.sourceType).toLowerCase()
  return (
    kind.startsWith('mcp:') ||
    sourceType === 'mcp' ||
    !!event.data?.mcpServerId ||
    toolName.startsWith('mcp__') ||
    toolName.includes('/')
  )
}

function resolveMember(event: AgentEvent, memberByRunId: Map<string, string>): string {
  const runId = clean(event.data?.runId)
  const fromRun = runId ? memberByRunId.get(runId) : ''
  return (
    fromRun ||
    clean(event.data?.memberLabel) ||
    clean(event.data?.member) ||
    clean(event.data?.memberId) ||
    'Unknown member'
  )
}

function toMcpToolCall(event: AgentEvent, memberByRunId: Map<string, string>): McpToolCall | null {
  if (!isMcpToolEvent(event)) return null
  const rawToolName = clean(event.data?.toolName ?? event.data?.toolNameRaw ?? event.title)
  if (!rawToolName) return null
  return {
    id: event.id,
    toolName: stripMcpToolName(rawToolName),
    server: inferServer(event, rawToolName),
    member: resolveMember(event, memberByRunId),
    status: event.status,
    startedAt: event.startedAt,
    completedAt: event.completedAt,
    error: clean(event.error || event.data?.error),
  }
}

function formatTime(iso: string): string {
  const date = new Date(iso)
  if (!Number.isFinite(date.getTime())) return 'Unknown time'
  return date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
}

function statusIcon(status: AgentEvent['status']) {
  if (status === 'error') return XCircle
  if (status === 'pending') return Clock
  return CheckCircle2
}

function statusClass(status: AgentEvent['status']): string {
  if (status === 'error') return 'text-[var(--red)] bg-[color-mix(in_srgb,var(--red)_10%,transparent)]'
  if (status === 'pending') return 'text-[var(--amber)] bg-[color-mix(in_srgb,var(--amber)_10%,transparent)]'
  return 'text-[var(--green)] bg-[color-mix(in_srgb,var(--green)_10%,transparent)]'
}

export default function SpaceInspectorTab({ spaceId }: SpaceInspectorTabProps) {
  const { query, inspectorEvents } = useSpaceDetail(spaceId)
  const rosterQuery = useSpaceRoster(spaceId)

  const memberByRunId = useMemo(() => {
    const map = new Map<string, string>()
    for (const member of rosterQuery.data?.members ?? []) {
      if (member.runId && member.memberLabel) map.set(member.runId, member.memberLabel)
    }
    return map
  }, [rosterQuery.data])

  const toolCalls = useMemo(
    () =>
      inspectorEvents
        .map((event) => toMcpToolCall(event, memberByRunId))
        .filter((call): call is McpToolCall => call !== null)
        .sort((left, right) => Date.parse(right.startedAt) - Date.parse(left.startedAt)),
    [inspectorEvents, memberByRunId],
  )

  return (
    <div className="flex h-full min-h-0 flex-col bg-[var(--bg-app)]">
      <div className="shrink-0 border-b border-[color-mix(in_srgb,var(--border)_45%,transparent)] px-8 py-5">
        <div className="flex items-center gap-3">
          <div className="flex h-9 w-9 items-center justify-center rounded-[8px] bg-[var(--bg-elevated)] text-[var(--text-2)]">
            <Server size={16} aria-hidden />
          </div>
          <div className="min-w-0">
            <h2 className="m-0 text-[15px] font-semibold leading-tight text-[var(--text-1)]">
              MCP Tool Calls
            </h2>
            <div className="mt-1 text-[12px] text-[var(--text-3)]">
              {toolCalls.length === 0
                ? 'No MCP tool calls recorded yet.'
                : `${toolCalls.length} call${toolCalls.length === 1 ? '' : 's'} recorded`}
            </div>
          </div>
        </div>
      </div>

      {query.isLoading ? (
        <div className="flex flex-1 min-h-0 flex-col items-center justify-center gap-3 text-[var(--text-3)]">
          <Activity size={18} className="animate-pulse opacity-50" aria-hidden />
          <span className="text-[12px]">Loading MCP activity...</span>
        </div>
      ) : toolCalls.length === 0 ? (
        <div className="flex flex-1 min-h-0 flex-col items-center justify-center gap-4 px-8 text-center">
          <div className="flex h-12 w-12 items-center justify-center rounded-[10px] border border-dashed border-[color-mix(in_srgb,var(--border)_65%,transparent)] text-[var(--text-3)]">
            <Search size={18} aria-hidden />
          </div>
          <div>
            <div className="text-[14px] font-semibold text-[var(--text-2)]">No MCP calls yet</div>
            <p className="mt-1 max-w-[320px] text-[12px] leading-relaxed text-[var(--text-3)]">
              Calls from connected harnesses will appear here with the member that made them.
            </p>
          </div>
        </div>
      ) : (
        <div className="flex-1 min-h-0 overflow-y-auto px-8 py-5">
          <div className="overflow-hidden rounded-[8px] border border-[color-mix(in_srgb,var(--border)_55%,transparent)] bg-[var(--bg-surface)]">
            <div className="grid grid-cols-[minmax(180px,1.4fr)_minmax(120px,0.8fr)_minmax(150px,1fr)_120px] gap-4 border-b border-[color-mix(in_srgb,var(--border)_45%,transparent)] px-4 py-2 text-[10px] font-semibold uppercase tracking-[0.08em] text-[var(--text-3)]">
              <span>Tool</span>
              <span>Server</span>
              <span>Member</span>
              <span>Time</span>
            </div>
            {toolCalls.map((call) => {
              const StatusIcon = statusIcon(call.status)
              return (
                <div
                  key={call.id}
                  className="grid grid-cols-[minmax(180px,1.4fr)_minmax(120px,0.8fr)_minmax(150px,1fr)_120px] gap-4 border-b border-[color-mix(in_srgb,var(--border)_30%,transparent)] px-4 py-3 last:border-b-0"
                >
                  <div className="min-w-0">
                    <div className="flex items-center gap-2">
                      <span className={cn('inline-flex h-5 w-5 items-center justify-center rounded-[5px]', statusClass(call.status))}>
                        <StatusIcon size={12} aria-hidden />
                      </span>
                      <span className="truncate font-mono text-[12px] text-[var(--text-1)]" title={call.toolName}>
                        {call.toolName}
                      </span>
                    </div>
                    {call.error && (
                      <div className="mt-1 truncate text-[11px] text-[var(--red)]" title={call.error}>
                        {call.error}
                      </div>
                    )}
                  </div>
                  <div className="truncate text-[12px] text-[var(--text-2)]" title={call.server}>
                    {call.server}
                  </div>
                  <div className="truncate text-[12px] text-[var(--text-2)]" title={call.member}>
                    {call.member}
                  </div>
                  <div className="text-[12px] tabular-nums text-[var(--text-3)]">
                    {formatTime(call.startedAt)}
                  </div>
                </div>
              )
            })}
          </div>
        </div>
      )}
    </div>
  )
}
