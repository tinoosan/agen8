import { useRef, useState, useEffect, useMemo } from 'react'
import { useStore } from '../lib/store'
import { useProjectTeams } from '../hooks/useProjectTeams'
import { useTeamStatus } from '../hooks/useTeamStatus'
import { useTeamManifest } from '../hooks/useTeamStatus'
import { useLogEvents } from '../hooks/useLogEvents'
import type { EventRecord } from '../lib/types'
import { ChevronRight, Search, ArrowDown, ArrowUp, Filter } from 'lucide-react'

/* ── Kind / type helpers (shared with ActivityFeed) ────── */

const typeCategories: Record<string, string[]> = {
  Task: ['task.queued', 'task.start', 'task.delegated', 'task.done', 'task.quarantined', 'task.create', 'task.claim'],
  Agent: ['agent.op.request', 'agent.op.response', 'agent.step', 'agent.error', 'agent_final', 'subagent.spawn', 'subagent.done'],
  LLM: ['llm.error', 'llm.retry', 'llm.usage.total', 'llm.cost.total', 'model_response', 'model.thinking.start', 'model.thinking.end'],
  System: ['daemon.start', 'daemon.stop', 'daemon.error', 'run.start', 'control.success', 'control.check', 'control.error', 'context.size', 'context.compacted'],
  IO: ['fs_read', 'fs_write', 'code_exec', 'code_compile', 'user_message', 'agent_message'],
}

const typeColors: Record<string, { bg: string; fg: string }> = {
  Task: { bg: 'var(--green-dim)', fg: 'var(--green)' },
  Agent: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  LLM: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  System: { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' },
  IO: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
}

function categorizeType(type: string): string {
  const lower = type.toLowerCase()
  for (const [cat, types] of Object.entries(typeCategories)) {
    if (types.some(t => lower.includes(t) || lower === t)) return cat
  }
  if (lower.includes('error') || lower.includes('fail')) return 'System'
  return 'System'
}

function isErrorEvent(event: EventRecord): boolean {
  const t = (event.type ?? '').toLowerCase()
  return t.includes('error') || t.includes('fail') || t.includes('quarantine')
}

/* ── Time formatting ──────────────────────────────────── */

function formatTime(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit', second: '2-digit' })
  } catch {
    return iso
  }
}

/* ── Team selector ────────────────────────────────────── */

function useTeamSessionId(teamId: string | null) {
  const manifestQuery = useTeamManifest(teamId)
  const focusedProjectRoot = useStore(s => s.focusedProjectRoot)
  const teams = useProjectTeams(focusedProjectRoot)
  const team = teams.data?.find(t => t.teamId === teamId)
  return team?.primarySessionId ?? manifestQuery.data?.coordinatorThreadId ?? null
}

/* ── Log row ──────────────────────────────────────────── */

function LogRow({ event, roleByRunId }: { event: EventRecord; roleByRunId: Record<string, string> }) {
  const [expanded, setExpanded] = useState(false)
  const role = roleByRunId[event.runId] || event.data?.role || event.origin || ''
  const isError = isErrorEvent(event)
  const category = categorizeType(event.type)
  const catColor = typeColors[category] ?? typeColors.System

  const hasData = !!(event.data && Object.keys(event.data).length > 0)

  return (
    <>
      <tr
        onClick={() => hasData && setExpanded(e => !e)}
        style={{
          cursor: hasData ? 'pointer' : 'default',
          borderLeft: isError ? '2px solid var(--red)' : '2px solid transparent',
          transition: 'background 0.1s',
        }}
        className="row-hover"
      >
        {/* Time */}
        <td style={{
          padding: '6px 10px', fontSize: 11, color: 'var(--text-3)',
          fontVariantNumeric: 'tabular-nums', whiteSpace: 'nowrap',
        }} className="mono">
          {event.timestamp ? formatTime(event.timestamp) : '—'}
        </td>

        {/* Role */}
        <td style={{ padding: '6px 10px' }}>
          {role && (
            <span style={{
              fontSize: 10, fontWeight: 600, color: 'var(--text-3)',
              textTransform: 'uppercase', letterSpacing: '0.04em',
            }}>
              {role}
            </span>
          )}
        </td>

        {/* Type pill */}
        <td style={{ padding: '6px 10px' }}>
          <span style={{
            fontSize: 9, fontWeight: 600, letterSpacing: '0.04em',
            textTransform: 'uppercase', padding: '1px 6px',
            borderRadius: 999, background: catColor.bg, color: catColor.fg,
            whiteSpace: 'nowrap',
          }}>
            {event.type.length > 20 ? event.type.slice(0, 20) + '…' : event.type}
          </span>
        </td>

        {/* Message */}
        <td style={{
          padding: '6px 10px', fontSize: 12,
          color: isError ? 'var(--red)' : 'var(--text-2)',
          maxWidth: 0,
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
            {hasData && (
              <ChevronRight size={10} style={{
                color: 'var(--text-3)', flexShrink: 0,
                transform: expanded ? 'rotate(90deg)' : 'none',
                transition: 'transform 0.15s',
              }} />
            )}
            <span className="truncate">{event.message || '—'}</span>
          </div>
        </td>
      </tr>

      {/* Expanded data */}
      {expanded && hasData && (
        <tr>
          <td colSpan={4} style={{ padding: '0 10px 8px 40px' }}>
            <div className="animate-fade-in" style={{
              background: 'var(--bg-surface)', borderRadius: 'var(--r-md)',
              padding: '8px 12px', fontSize: 11, color: 'var(--text-2)',
            }}>
              <div className="mono" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 300, overflowY: 'auto' }}>
                {JSON.stringify(event.data, null, 2)}
              </div>
              {event.origin && (
                <div style={{ marginTop: 6, fontSize: 10, color: 'var(--text-3)' }}>
                  Origin: {event.origin} &middot; Run: {event.runId.slice(0, 12)}
                </div>
              )}
            </div>
          </td>
        </tr>
      )}
    </>
  )
}

/* ── Main Logs Page ───────────────────────────────────── */

export default function Logs() {
  const focusedProjectRoot = useStore(s => s.focusedProjectRoot)
  const teamsQuery = useProjectTeams(focusedProjectRoot)
  const teams = teamsQuery.data ?? []

  // Pick first team by default
  const [selectedTeamId, setSelectedTeamId] = useState<string | null>(null)
  const teamId = selectedTeamId ?? teams[0]?.teamId ?? null

  const sessionId = useTeamSessionId(teamId)
  const statusQuery = useTeamStatus(teamId)
  const roleByRunId = statusQuery.data?.roleByRunId ?? {}
  const availableRoles = useMemo(() => {
    const roles = new Set<string>()
    for (const role of Object.values(roleByRunId)) {
      if (role) roles.add(role)
    }
    return [...roles].sort()
  }, [roleByRunId])

  // Filters
  const [search, setSearch] = useState('')
  const [roleFilter, setRoleFilter] = useState<string>('')
  const [categoryFilter, setCategoryFilter] = useState<string>('')
  const [errorsOnly, setErrorsOnly] = useState(false)
  const [sortDesc, setSortDesc] = useState(false)

  // Find runId for role filter
  const filterRunId = useMemo(() => {
    if (!roleFilter) return null
    for (const [runId, role] of Object.entries(roleByRunId)) {
      if (role === roleFilter) return runId
    }
    return null
  }, [roleFilter, roleByRunId])

  const logQuery = useLogEvents({
    sessionId: filterRunId ? undefined : sessionId,
    runId: filterRunId ?? undefined,
    limit: 500,
    sortDesc,
  })
  const events = logQuery.data ?? []

  // Client-side filtering
  const filtered = useMemo(() => {
    let result = events
    if (errorsOnly) {
      result = result.filter(isErrorEvent)
    }
    if (categoryFilter) {
      const catTypes = typeCategories[categoryFilter] ?? []
      result = result.filter(e => {
        const lower = e.type.toLowerCase()
        return catTypes.some(t => lower.includes(t) || lower === t)
      })
    }
    if (search) {
      const q = search.toLowerCase()
      result = result.filter(e =>
        e.message?.toLowerCase().includes(q) ||
        e.type?.toLowerCase().includes(q) ||
        (roleByRunId[e.runId] ?? '').toLowerCase().includes(q)
      )
    }
    return result
  }, [events, errorsOnly, categoryFilter, search, roleByRunId])

  // Auto-scroll
  const containerRef = useRef<HTMLDivElement>(null)
  const [autoScroll, setAutoScroll] = useState(true)

  useEffect(() => {
    if (!autoScroll) return
    const el = containerRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [filtered.length, autoScroll])

  function handleScroll() {
    const el = containerRef.current
    if (!el) return
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 60
    setAutoScroll(atBottom)
  }

  // Auto-select first team
  useEffect(() => {
    if (!selectedTeamId && teams.length > 0) {
      setSelectedTeamId(teams[0].teamId)
    }
  }, [selectedTeamId, teams])

  const categories = Object.keys(typeCategories)

  return (
    <div style={{ height: '100%', display: 'flex', flexDirection: 'column', padding: '24px 32px 16px' }}>
      {/* Header */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16, flexShrink: 0 }}>
        <h1 style={{ margin: 0, fontSize: 22, fontWeight: 700, letterSpacing: '-0.04em', color: 'var(--text-1)' }}>
          Logs
        </h1>
        <div style={{ flex: 1 }} />
        {/* Team selector */}
        {teams.length > 1 && (
          <select
            value={teamId ?? ''}
            onChange={e => setSelectedTeamId(e.target.value || null)}
            style={{
              background: 'var(--bg-elevated)', border: '1px solid var(--border)',
              borderRadius: 'var(--r-md)', padding: '4px 8px', fontSize: 12,
              color: 'var(--text-2)', fontFamily: 'inherit',
            }}
          >
            {teams.map(t => (
              <option key={t.teamId} value={t.teamId}>
                {t.profileId ?? t.teamId.slice(0, 12)}
              </option>
            ))}
          </select>
        )}
        <span style={{ fontSize: 11, color: 'var(--text-3)', fontVariantNumeric: 'tabular-nums' }}>
          {filtered.length} event{filtered.length !== 1 ? 's' : ''}
        </span>
      </div>

      {/* Filter bar */}
      <div style={{
        display: 'flex', alignItems: 'center', gap: 8, marginBottom: 12, flexShrink: 0, flexWrap: 'wrap',
      }}>
        {/* Search */}
        <div style={{
          display: 'flex', alignItems: 'center', gap: 6,
          background: 'var(--bg-elevated)', border: '1px solid var(--border)',
          borderRadius: 'var(--r-md)', padding: '5px 10px', flex: '1 1 200px', maxWidth: 320,
        }}>
          <Search size={12} style={{ color: 'var(--text-3)', flexShrink: 0 }} />
          <input
            type="text"
            value={search}
            onChange={e => setSearch(e.target.value)}
            placeholder="Search logs..."
            style={{
              background: 'transparent', border: 'none', outline: 'none',
              color: 'var(--text-1)', fontSize: 12, fontFamily: 'inherit', width: '100%',
            }}
          />
        </div>

        {/* Role filter */}
        {availableRoles.length > 0 && (
          <select
            value={roleFilter}
            onChange={e => setRoleFilter(e.target.value)}
            style={{
              background: 'var(--bg-elevated)', border: '1px solid var(--border)',
              borderRadius: 'var(--r-md)', padding: '5px 8px', fontSize: 11,
              color: roleFilter ? 'var(--text-1)' : 'var(--text-3)', fontFamily: 'inherit',
            }}
          >
            <option value="">All roles</option>
            {availableRoles.map(r => (
              <option key={r} value={r}>{r}</option>
            ))}
          </select>
        )}

        {/* Category buttons */}
        <div style={{ display: 'flex', gap: 4 }}>
          {categories.map(cat => {
            const active = categoryFilter === cat
            const colors = typeColors[cat] ?? typeColors.System
            return (
              <button
                key={cat}
                onClick={() => setCategoryFilter(active ? '' : cat)}
                style={{
                  padding: '3px 8px', borderRadius: 'var(--r-sm)', border: 'none',
                  fontSize: 10, fontWeight: 600, cursor: 'pointer',
                  background: active ? colors.bg : 'transparent',
                  color: active ? colors.fg : 'var(--text-3)',
                  transition: 'all 0.15s',
                }}
              >
                {cat}
              </button>
            )
          })}
        </div>

        {/* Errors only */}
        <button
          onClick={() => setErrorsOnly(e => !e)}
          style={{
            display: 'flex', alignItems: 'center', gap: 4,
            padding: '3px 8px', borderRadius: 'var(--r-sm)', border: 'none',
            fontSize: 10, fontWeight: 600, cursor: 'pointer',
            background: errorsOnly ? 'var(--red-dim)' : 'transparent',
            color: errorsOnly ? 'var(--red)' : 'var(--text-3)',
            transition: 'all 0.15s',
          }}
        >
          <Filter size={10} />
          Errors
        </button>
      </div>

      {/* Table */}
      <div
        ref={containerRef}
        onScroll={handleScroll}
        style={{
          flex: 1, minHeight: 0, overflowY: 'auto',
          background: 'var(--bg-panel)', border: '1px solid var(--border)',
          borderRadius: 'var(--r-lg)',
        }}
      >
        {!sessionId && !logQuery.isLoading ? (
          <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
            No teams running. Start a team to see logs.
          </div>
        ) : logQuery.isLoading ? (
          <div style={{ padding: 40, textAlign: 'center' }}>
            <span className="spinner spinner-md" />
          </div>
        ) : filtered.length === 0 ? (
          <div style={{ padding: 40, textAlign: 'center', color: 'var(--text-3)', fontSize: 13 }}>
            {events.length > 0 ? 'No events match filters' : 'No events yet'}
          </div>
        ) : (
          <table style={{ width: '100%', borderCollapse: 'collapse' }}>
            <thead style={{ position: 'sticky', top: 0, background: 'var(--bg-panel)', zIndex: 1 }}>
              <tr style={{ borderBottom: '1px solid var(--border)' }}>
                <th
                  onClick={() => { setSortDesc(d => !d); if (!sortDesc) setAutoScroll(false) }}
                  style={{
                    padding: '8px 10px', fontSize: 9, fontWeight: 600,
                    textTransform: 'uppercase', letterSpacing: '0.06em',
                    color: 'var(--text-3)', textAlign: 'left',
                    borderBottom: '1px solid var(--border)',
                    cursor: 'pointer', userSelect: 'none',
                    display: 'flex', alignItems: 'center', gap: 4,
                  }}
                >
                  Time
                  {sortDesc ? <ArrowDown size={10} /> : <ArrowUp size={10} />}
                </th>
                {['Role', 'Type', 'Message'].map(h => (
                  <th key={h} style={{
                    padding: '8px 10px', fontSize: 9, fontWeight: 600,
                    textTransform: 'uppercase', letterSpacing: '0.06em',
                    color: 'var(--text-3)', textAlign: 'left',
                    borderBottom: '1px solid var(--border)',
                  }}>
                    {h}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {filtered.map(event => (
                <LogRow key={event.eventId} event={event} roleByRunId={roleByRunId} />
              ))}
            </tbody>
          </table>
        )}
      </div>

      {/* Jump to latest */}
      {!autoScroll && filtered.length > 0 && (
        <button
          onClick={() => {
            setAutoScroll(true)
            const el = containerRef.current
            if (el) el.scrollTop = el.scrollHeight
          }}
          style={{
            position: 'fixed', bottom: 24, right: 40,
            display: 'flex', alignItems: 'center', gap: 5,
            padding: '6px 12px', borderRadius: 'var(--r-md)',
            background: 'var(--accent-solid)', color: '#fff',
            border: 'none', fontSize: 11, fontWeight: 600,
            cursor: 'pointer', boxShadow: '0 4px 16px rgba(0,0,0,0.3)',
            zIndex: 20,
          }}
        >
          <ArrowDown size={12} />
          Latest
        </button>
      )}
    </div>
  )
}
