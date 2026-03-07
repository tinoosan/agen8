import { useRef, useState, useEffect, useMemo } from 'react'
import { useActivity } from '../hooks/useActivity'
import { useStore } from '../lib/store'
import type { ActivityEvent } from '../lib/types'
import { Prism as SyntaxHighlighter } from 'react-syntax-highlighter'
import { vscDarkPlus, vs } from 'react-syntax-highlighter/dist/esm/styles/prism'
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
  if (lower.includes('_') || lower.includes('.')) return null
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

/* ── Formatting helpers ──────────────────────────────── */

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

function formatDuration(ms: number): string {
  if (ms < 1000) return `${ms}ms`
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)}s`
  return `${(ms / 60_000).toFixed(1)}m`
}

/** Extract the basename from a file path */
function basename(path: string): string {
  const parts = path.split('/')
  return parts[parts.length - 1] || path
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

function renderJSONOrText(data: unknown): string {
  if (typeof data === 'string') return data
  return JSON.stringify(data, null, 2)
}

function EventRow({ event }: { event: ActivityEvent }) {
  const [expanded, setExpanded] = useState(false)
  const { theme } = useStore()
  const eventRole = event.data?.role || event.data?.agent_role || ''
  const message = event.title || event.outputPreview || event.textPreview || ''

  // Hide thinking events entirely from activity feed
  if (event.kind?.startsWith('model.thinking')) {
    return null
  }

  const summary = message || event.kind || ''
  const role = eventRole
  const statusClass = getStatusClass(event)
  const isError = event.status === 'error' || event.kind === 'error'
  const isPending = statusClass === 'pending'
  const kindLower = (event.kind ?? '').toLowerCase()

  // Inline context: file path, command, spawned role, error
  const inlinePath = event.path ? basename(event.path) : null
  const inlineCommand = (kindLower === 'code_exec' || kindLower.includes('exec')) ? (event.data?.command || event.data?.cmd) : null
  const inlineExitCode = event.data?.exit_code ?? event.data?.exitCode
  const inlineSpawnRole = kindLower.includes('spawn') ? (event.data?.spawned_role || event.data?.role_name || event.data?.spawnedRole) : null
  const inlineError = isError ? (event.error || event.data?.error) : null

  // Duration
  const duration = event.duration ?? (event.finishedAt && event.startedAt
    ? new Date(event.finishedAt).getTime() - new Date(event.startedAt).getTime()
    : null)

  // Theme-aware syntax style
  const syntaxStyle = theme === 'light' ? vs : vscDarkPlus

  // Build a rich detail object of all interesting payload fields to display when expanded.
  const detailsList = useMemo(() => {
    const d: Record<string, React.ReactNode> = {}

    // Add Syntax Highlighting for code_exec
    if (event.kind === 'code_exec' && event.data?.code) {
      d['Code Executed'] = (
        <SyntaxHighlighter
          language={event.data?.language || 'javascript'}
          style={syntaxStyle}
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
    if (event.error) d['Error'] = <span className="mono" style={{ color: 'var(--red)' }}>{event.error}</span>
    return d
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [event, syntaxStyle])

  const hasDetail = Object.keys(detailsList).length > 0

  return (
    <div
      className={`activity-row${hasDetail ? ' has-detail' : ''}${isError ? ' is-error' : ''}`}
      onClick={() => hasDetail && setExpanded(e => !e)}
    >
      {/* Status indicator */}
      {isPending ? (
        <span className="spinner" style={{
          width: 6, height: 6, flexShrink: 0, marginTop: 5,
          borderWidth: 1.5,
        }} />
      ) : (
        <div className={`status-dot ${statusClass}`} />
      )}

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

          {/* Inline file path */}
          {inlinePath && (
            <span className="mono" style={{ fontSize: 10, color: 'var(--text-3)', flexShrink: 0 }}>
              {inlinePath}
            </span>
          )}

          {/* Inline command for exec events */}
          {inlineCommand && (
            <span className="mono truncate" style={{ fontSize: 10, color: 'var(--text-2)', maxWidth: 160 }}>
              {inlineCommand}
            </span>
          )}

          {/* Inline exit code */}
          {inlineExitCode != null && (
            <span className="mono" style={{
              fontSize: 9, flexShrink: 0, fontWeight: 600,
              color: String(inlineExitCode) === '0' ? 'var(--green)' : 'var(--red)',
            }}>
              {String(inlineExitCode) === '0' ? 'ok' : `exit ${inlineExitCode}`}
            </span>
          )}

          {/* Inline spawned role */}
          {inlineSpawnRole && (
            <span style={{ fontSize: 11, color: 'var(--accent)', fontWeight: 500 }}>
              {inlineSpawnRole}
            </span>
          )}

          {/* Summary text */}
          <span className="truncate" style={{ fontSize: 12, color: 'var(--text-2)', flex: 1 }}>
            {summary}
          </span>

          {/* Inline error (red, visible without expanding) */}
          {inlineError && !expanded && (
            <span className="truncate" style={{ fontSize: 11, color: 'var(--red)', maxWidth: 200, flexShrink: 1 }}>
              {inlineError}
            </span>
          )}

          {/* Duration */}
          {duration != null && duration > 0 && (
            <span style={{
              fontSize: 10, color: 'var(--text-3)',
              fontVariantNumeric: 'tabular-nums',
              flexShrink: 0,
            }}>
              {formatDuration(duration)}
            </span>
          )}

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
  const query = useActivity({ threadId, teamId, includeChildRuns: true, limit: 200 })
  const containerRef = useRef<HTMLDivElement>(null)
  const bottomRef = useRef<HTMLDivElement>(null)
  const events = query.data ?? []
  const [isAtBottom, setIsAtBottom] = useState(true)
  const [hasNew, setHasNew] = useState(false)
  const prevLenRef = useRef(0)

  const feed = useMemo(() => buildFeed(events), [events])

  // Track scroll position — are we near the bottom?
  useEffect(() => {
    const el = containerRef.current
    if (!el) return
    function handleScroll() {
      if (!el) return
      const threshold = 60
      const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < threshold
      setIsAtBottom(atBottom)
      if (atBottom) setHasNew(false)
    }
    el.addEventListener('scroll', handleScroll, { passive: true })
    return () => el.removeEventListener('scroll', handleScroll)
  }, [])

  // Auto-scroll when new events arrive (only if user is at bottom)
  useEffect(() => {
    if (feed.length > prevLenRef.current) {
      if (isAtBottom) {
        bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
      } else {
        setHasNew(true)
      }
    }
    prevLenRef.current = feed.length
  }, [feed.length, isAtBottom])

  // Initial scroll to bottom on mount
  useEffect(() => {
    const el = containerRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [])

  function scrollToBottom() {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
    setHasNew(false)
  }

  return (
    <div
      ref={containerRef}
      style={{ overflowY: 'auto', flex: 1, minHeight: 0, position: 'relative' }}
    >
      {events.length === 0 ? (
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
        <>
          {feed.map((entry) =>
            entry.type === 'divider' ? (
              <div className="time-divider" key={entry.key}>
                <span>{entry.label}</span>
              </div>
            ) : (
              <EventRow key={entry.event.id} event={entry.event} />
            )
          )}
          <div ref={bottomRef} />
        </>
      )}

      {/* Jump-to-bottom button */}
      {hasNew && !isAtBottom && (
        <button
          onClick={scrollToBottom}
          className="animate-fade-in"
          style={{
            position: 'sticky', bottom: 8,
            left: '50%', transform: 'translateX(-50%)',
            display: 'flex', alignItems: 'center', gap: 5,
            padding: '5px 14px', borderRadius: 999,
            border: '1px solid var(--border)',
            background: 'var(--bg-panel)',
            boxShadow: '0 2px 12px rgba(0,0,0,0.15)',
            color: 'var(--accent)', fontSize: 11, fontWeight: 600,
            fontFamily: 'inherit', cursor: 'pointer',
            zIndex: 5,
            transition: 'background 0.12s',
          }}
        >
          ↓ New activity
        </button>
      )}
    </div>
  )
}
