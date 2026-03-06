import { useRef, useState, useEffect, useMemo } from 'react'
import { useActivity } from '../hooks/useActivity'
import type { ActivityEvent } from '../lib/types'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus } from 'react-syntax-highlighter/dist/esm/styles/prism'
import { ChevronRight, Activity } from 'lucide-react'

interface ActivityFeedProps {
  threadId: string | null
  teamId: string
}

/* ── Kind label helpers ──────────────────────────────── */

const kindColors: Record<string, { bg: string; fg: string }> = {
  error: { bg: 'var(--red-dim)', fg: 'var(--red)' },
  fail: { bg: 'var(--red-dim)', fg: 'var(--red)' },
  done: { bg: 'var(--green-dim)', fg: 'var(--green)' },
  complete: { bg: 'var(--green-dim)', fg: 'var(--green)' },
  start: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  tool: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  read: { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' },
  write: { bg: 'var(--amber-dim)', fg: 'var(--amber)' },
  exec: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  spawn: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  message: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  model: { bg: 'var(--accent-dim)', fg: 'var(--accent)' },
  task: { bg: 'var(--green-dim)', fg: 'var(--green)' },
}

/** Map raw internal kind strings to short, human-friendly labels. Returns null to hide the pill. */
function humanizeKind(kind: string): string | null {
  const lower = kind.toLowerCase()
  // known human-friendly mappings
  const map: Record<string, string> = {
    'fs_read': 'Read',
    'fs_write': 'Write',
    'fs_list': 'List',
    'fs_stat': 'Stat',
    'fs_delete': 'Delete',
    'fs_mkdir': 'Mkdir',
    'fs_move': 'Move',
    'fs_replace': 'Replace',
    'fs_search': 'Search',
    'code_exec': 'Exec',
    'code_compile': 'Compile',
    'tool_execution': 'Tool',
    'user_message': 'Message',
    'agent_message': 'Reply',
    'agent_speak': 'Reply',
    'model_response': 'Response',
    'task.done': 'Done',
    'task.start': 'Start',
    'task.create': 'Task',
    'task.claim': 'Claimed',
    'subagent.spawn': 'Spawn',
    'subagent.done': 'Done',
    'error': 'Error',
  }
  if (map[lower]) return map[lower]
  // fallback: if it contains underscores or dots, it's likely an internal name — hide it
  if (lower.includes('_') || lower.includes('.')) return null
  // short labels like "done", "start" are fine as-is
  if (kind.length <= 8) return kind
  return null
}

function getKindStyle(kind: string): { bg: string; fg: string } {
  const lower = kind.toLowerCase()
  for (const [key, style] of Object.entries(kindColors)) {
    if (lower.includes(key)) return style
  }
  return { bg: 'var(--bg-elevated)', fg: 'var(--text-3)' }
}

function getStatusClass(event: ActivityEvent): string {
  if (event.status === 'error' || event.error) return 'error'
  if (event.status === 'pending' || !event.finishedAt) return 'pending'
  return 'ok'
}

/* ── Relative time ──────────────────────────────────── */

function relativeTime(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 0) return 'now'
  const s = Math.floor(diff / 1000)
  if (s < 5) return 'now'
  if (s < 60) return `${s}s`
  const m = Math.floor(s / 60)
  if (m < 60) return `${m}m`
  const h = Math.floor(m / 60)
  if (h < 24) return `${h}h`
  return `${Math.floor(h / 24)}d`
}

/* ── Time gap detection for dividers ────────────────── */

interface FeedItem {
  type: 'event'
  event: ActivityEvent
}
interface FeedDivider {
  type: 'divider'
  label: string
  key: string
}
type FeedEntry = FeedItem | FeedDivider

const GAP_THRESHOLD_MS = 30_000

function buildFeed(events: ActivityEvent[]): FeedEntry[] {
  const result: FeedEntry[] = []
  let lastTime: number | null = null

  for (const event of events) {
    const t = event.startedAt ? new Date(event.startedAt).getTime() : null

    if (t && lastTime && t - lastTime > GAP_THRESHOLD_MS) {
      const date = new Date(t)
      const label = date.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' })
      result.push({ type: 'divider', label, key: `div-${t}` })
    }

    result.push({ type: 'event', event })
    if (t) lastTime = t
  }

  return result
}

/* ── Single event row ───────────────────────────────── */

function renderJSONOrText(data: any): string {
  if (typeof data === 'string') return data
  return JSON.stringify(data, null, 2)
}

function EventRow({ event }: { event: ActivityEvent }) {
  const [expanded, setExpanded] = useState(false)
  const eventRole = event.data?.role || event.data?.agent_role || ''
  const message = event.title || event.outputPreview || event.textPreview || ''

  // Handle specialized "Thinking" blocks
  if (event.kind === 'model.thinking.summary') {
    return (
      <div
        className="activity-row"
        style={{
          background: 'rgba(255, 255, 255, 0.015)',
          borderLeft: '2px solid var(--accent)',
          borderTop: '1px solid rgba(255, 255, 255, 0.03)',
          borderRight: '1px solid rgba(255, 255, 255, 0.03)',
          borderBottom: '1px solid rgba(255, 255, 255, 0.03)',
          marginBottom: 6,
        }}
      >
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0, width: '100%' }}>
          {/* Animated thinking pulse */}
          <div style={{
            width: 6, height: 6, borderRadius: '50%',
            background: 'var(--accent)',
            animation: 'pulse-soft 2s infinite',
            marginLeft: 4,
          }} />

          {/* Role text label */}
          {eventRole && (
            <span style={{ fontSize: 10, fontWeight: 600, color: 'var(--text-3)', letterSpacing: '0.04em', textTransform: 'uppercase', flexShrink: 0 }}>
              {eventRole}
            </span>
          )}

          <span className="truncate" style={{ fontSize: 12, fontWeight: 500, color: 'var(--text-1)', fontStyle: 'italic', flexShrink: 0, maxWidth: 300 }}>
            {message || 'Thinking...'}
          </span>

          {/* Optional extracted text */}
          {event.data?.text && (
            <span className="truncate" style={{ fontSize: 12, color: 'var(--text-3)', fontStyle: 'italic', flex: 1 }}>
              {event.data.text}
            </span>
          )}

          {/* Relative timestamp */}
          {event.startedAt && (
            <span style={{
              fontSize: 10, color: 'var(--text-4)',
              fontVariantNumeric: 'tabular-nums',
              flexShrink: 0,
            }}>
              {relativeTime(event.startedAt)}
            </span>
          )}
        </div>
      </div>
    )
  }

  // Filter out the noisy start/end thinking events
  if (event.kind === 'model.thinking.start' || event.kind === 'model.thinking.end') {
    return null
  }

  const summary = message || event.kind || ''
  const role = eventRole
  const statusClass = getStatusClass(event)
  const isError = event.status === 'error' || event.kind === 'error'

  // Build a rich detail object of all interesting payload fields to display when expanded.
  const detailsList = useMemo(() => {
    const d: Record<string, React.ReactNode> = {}

    // Add Syntax Highlighting for code_exec
    if (event.kind === 'code_exec' && event.data?.code) {
      d['Code Executed'] = (
        <SyntaxHighlighter
          language={event.data?.language || 'javascript'}
          style={vscDarkPlus}
          customStyle={{ margin: 0, padding: '12px', fontSize: '11px', borderRadius: '4px', background: 'rgba(0,0,0,0.2)' }}
        >
          {event.data.code}
        </SyntaxHighlighter>
      )
    }

    if (event.data) {
      const remainingData = { ...event.data }
      delete remainingData.role
      delete remainingData.agent_role
      // Don't duplicate the code payload if we already rendered it
      if (event.kind === 'code_exec') delete remainingData.code

      if (Object.keys(remainingData).length > 0) {
        d['Data payload'] = <span className="mono">{renderJSONOrText(remainingData)}</span>
      }
    }

    if (event.path) d['Path'] = <span className="mono">{event.path}</span>
    if (event.outputPreview) d['Output'] = <span className="mono">{event.outputPreview}</span>
    if (event.error) d['Error'] = <span className="mono">{event.error}</span>
    return d
  }, [event])

  const hasDetail = Object.keys(detailsList).length > 0

  return (
    <div
      className={`activity-row${hasDetail ? ' has-detail' : ''}${isError ? ' is-error' : ''}`}
      onClick={() => hasDetail && setExpanded(e => !e)}
    >
      {/* Status dot */}
      <div className={`status-dot ${statusClass}`} />

      {/* Content */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6, minWidth: 0 }}>
          {/* Expand chevron */}
          {hasDetail ? (
            <ChevronRight
              size={10}
              style={{
                color: 'var(--text-3)',
                flexShrink: 0,
                transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
                transition: 'transform 0.15s',
              }}
            />
          ) : (
            <span style={{ width: 10, flexShrink: 0 }} />
          )}

          {/* Role text label */}
          {role && (
            <span style={{ fontSize: 10, fontWeight: 600, color: 'var(--text-3)', letterSpacing: '0.04em', textTransform: 'uppercase', flexShrink: 0 }}>
              {role}
            </span>
          )}

          {/* Kind pill */}
          {event.kind && (() => {
            const label = humanizeKind(event.kind)
            if (!label) return null
            const style = getKindStyle(event.kind)
            return (
              <span
                className="kind-pill"
                style={{ background: style.bg, color: style.fg }}
              >
                {label}
              </span>
            )
          })()}

          {/* Summary text */}
          <span className="truncate" style={{ fontSize: 12, color: 'var(--text-2)', flex: 1 }}>
            {summary}
          </span>

          {/* Relative timestamp */}
          {event.startedAt && (
            <span style={{
              fontSize: 10, color: 'var(--text-3)',
              fontVariantNumeric: 'tabular-nums',
              flexShrink: 0,
              marginLeft: 4,
            }}>
              {relativeTime(event.startedAt)}
            </span>
          )}
        </div>

        {/* Expanded detail */}
        {expanded && hasDetail && (
          <div
            className="animate-fade-in"
            style={{
              marginTop: 6,
              background: 'var(--bg-app)',
              borderRadius: 'var(--r-md)',
              border: '1px solid var(--border)',
              overflow: 'hidden',
            }}
          >
            {Object.entries(detailsList).map(([key, val], i) => (
              <div key={key} style={{
                borderTop: i > 0 ? '1px solid var(--border)' : 'none',
                padding: '6px 10px',
              }}>
                <div style={{ fontSize: 9, fontWeight: 600, color: 'var(--text-3)', textTransform: 'uppercase', letterSpacing: '0.04em', marginBottom: 4 }}>
                  {key}
                </div>
                <div
                  style={{
                    fontSize: 11,
                    color: 'var(--text-2)',
                    whiteSpace: 'pre-wrap',
                    wordBreak: 'break-word',
                    maxHeight: 250,
                    overflowY: 'auto',
                  }}
                >
                  {val}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </div>
  )
}

/* ── Main Feed ──────────────────────────────────────── */

export default function ActivityFeed({ threadId, teamId }: ActivityFeedProps) {
  const query = useActivity({ threadId, teamId, includeChildRuns: true, limit: 100 })
  const containerRef = useRef<HTMLDivElement>(null)
  const events = query.data ?? []
  const recent = events.slice(-50)

  const feed = useMemo(() => buildFeed(recent), [recent])

  // Auto-scroll to latest event
  useEffect(() => {
    const el = containerRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [feed.length])

  return (
    <div
      ref={containerRef}
      style={{ overflowY: 'auto', flex: 1, minHeight: 0 }}
    >
      {recent.length === 0 ? (
        <div className="activity-empty">
          <div className="empty-icon">
            <Activity size={16} style={{ color: 'var(--text-3)' }} />
          </div>
          <div className="empty-text">
            Waiting for activity…<br />
            Events will appear here in real time
          </div>
        </div>
      ) : (
        feed.map((entry) =>
          entry.type === 'divider' ? (
            <div className="time-divider" key={entry.key}>
              <span>{entry.label}</span>
            </div>
          ) : (
            <EventRow key={entry.event.id} event={entry.event} />
          )
        )
      )}
    </div>
  )
}
