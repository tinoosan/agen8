import { useRef, useEffect, useMemo, useState } from 'react'
import { useActivity } from '../hooks/useActivity'
import { useThinkingEvents } from '../hooks/useThinkingEvents'
import { useStore } from '../lib/store'
import type { ActivityEvent, EventRecord } from '../lib/types'
import { ChevronLeft, ChevronRight, Activity, Cpu, Coins, Sparkles } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'

interface RoleTranscriptProps {
  teamId: string
  threadId: string | null
  roleName: string
  runId: string | null
  model?: string
  tokens?: number
  cost?: number
  status?: string
}

/* ── Thinking entry ──────────────────────────────── */

interface ThinkingEntry {
  id: string
  text: string
  live: boolean
  createdAt: number
}

function thinkingEventsToEntries(events: EventRecord[]): ThinkingEntry[] {
  if (events.length === 0) return []
  const order = [...events].sort((a, b) => {
    const ts = new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime()
    if (ts !== 0) return ts
    return a.eventId.localeCompare(b.eventId)
  })

  const entries: ThinkingEntry[] = []
  let current: ThinkingEntry | null = null
  let blockIndex = 0

  for (const ev of order) {
    const type = (ev.type ?? '').trim()
    const step = (ev.data?.step ?? '').trim()
    if (!step) continue

    if (type === 'model.thinking.start') {
      if (current) {
        if (!current.text.trim()) current.text = 'Thinking\u2026'
        entries.push(current)
      }
      blockIndex++
      current = {
        id: `thinking:${step}:${blockIndex}`,
        text: '',
        live: true,
        createdAt: new Date(ev.timestamp).getTime(),
      }
      continue
    }
    if (!current) continue
    if (type === 'model.thinking.summary') {
      const line = (ev.data?.text ?? '').trim()
      if (line) current.text = current.text ? `${current.text}\n${line}` : line
      continue
    }
    if (type === 'model.thinking.end') {
      if (!current.text.trim()) current.text = 'Reasoning used; provider did not return a summary.'
      current.live = false
      entries.push(current)
      current = null
    }
  }
  if (current) {
    if (!current.text.trim()) current.text = 'Thinking\u2026'
    entries.push(current)
  }
  return entries.filter(e => e.text.trim() !== '')
}

/* ── Mini thinking bubble ────────────────────────── */

function MiniThought({ entry }: { entry: ThinkingEntry }) {
  const [open, setOpen] = useState(false)
  return (
    <div style={{ margin: '8px 0' }}>
      <button
        onClick={() => setOpen(o => !o)}
        style={{
          display: 'flex', alignItems: 'center', gap: 5,
          background: 'none', border: 'none', cursor: 'pointer',
          padding: 0, color: entry.live ? 'var(--amber)' : 'var(--text-3)',
          fontSize: 11, fontFamily: 'inherit',
        }}
      >
        <Sparkles size={11} />
        <ChevronRight size={10} style={{
          transform: open ? 'rotate(90deg)' : 'none',
          transition: 'transform 0.15s',
        }} />
        <span style={{ fontWeight: 600, letterSpacing: '0.04em', textTransform: 'uppercase' }}>
          Thinking
        </span>
        {entry.live && (
          <span style={{
            fontSize: 9, fontWeight: 600, color: 'var(--amber)',
            background: 'var(--amber-dim)', padding: '1px 5px', borderRadius: 999,
          }}>
            live
          </span>
        )}
      </button>
      {open && (
        <div
          className={entry.live ? 'thinking-shimmer-bg animate-fade-in' : 'animate-fade-in'}
          style={{
            marginTop: 4, padding: '8px 12px',
            borderLeft: `2px solid ${entry.live ? 'var(--amber)' : 'var(--border)'}`,
            background: entry.live ? undefined : 'var(--bg-surface)',
            borderRadius: '4px 8px 8px 8px',
            fontSize: 12, color: 'var(--text-2)', fontStyle: 'italic',
          }}
        >
          <div className="md-prose">
            <ReactMarkdown remarkPlugins={[remarkGfm]}>{entry.text}</ReactMarkdown>
          </div>
        </div>
      )}
    </div>
  )
}

/* ── Activity row (simplified) ───────────────────── */

function ActivityRow({ event }: { event: ActivityEvent }) {
  const [expanded, setExpanded] = useState(false)
  const kind = event.kind ?? ''
  const title = event.title || event.outputPreview || event.textPreview || kind
  const isError = event.status === 'error' || !!event.error
  const isPending = !event.finishedAt && event.status !== 'error'
  const hasDetail = !!(event.data && Object.keys(event.data).length > 0) || !!event.path || !!event.error

  // Skip thinking events
  if (kind.startsWith('model.thinking')) return null

  const durationMs = event.duration ?? (event.finishedAt && event.startedAt
    ? new Date(event.finishedAt).getTime() - new Date(event.startedAt).getTime()
    : null)

  return (
    <div
      onClick={() => hasDetail && setExpanded(e => !e)}
      style={{
        display: 'flex', alignItems: 'flex-start', gap: 8,
        padding: '6px 0',
        cursor: hasDetail ? 'pointer' : 'default',
        borderLeft: isError ? '2px solid var(--red)' : '2px solid transparent',
        paddingLeft: 8,
        transition: 'background 0.1s',
      }}
      className="row-hover"
    >
      {/* Status */}
      {isPending ? (
        <span className="spinner" style={{ width: 6, height: 6, borderWidth: 1.5, marginTop: 5, flexShrink: 0 }} />
      ) : (
        <span style={{
          width: 6, height: 6, borderRadius: '50%', flexShrink: 0, marginTop: 5,
          background: isError ? 'var(--red)' : 'var(--green)',
        }} />
      )}

      {/* Content */}
      <div style={{ flex: 1, minWidth: 0 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 6 }}>
          {hasDetail && (
            <ChevronRight size={10} style={{
              color: 'var(--text-3)', flexShrink: 0,
              transform: expanded ? 'rotate(90deg)' : 'none',
              transition: 'transform 0.15s',
            }} />
          )}
          {kind && (
            <span style={{
              fontSize: 9, fontWeight: 600, textTransform: 'uppercase',
              letterSpacing: '0.04em', padding: '1px 5px', borderRadius: 999,
              background: 'var(--bg-elevated)', color: 'var(--text-3)',
              flexShrink: 0,
            }}>
              {kind.length > 15 ? kind.slice(0, 15) + '…' : kind}
            </span>
          )}
          <span className="truncate" style={{ fontSize: 12, color: isError ? 'var(--red)' : 'var(--text-2)' }}>
            {title}
          </span>
          {durationMs != null && durationMs > 0 && (
            <span style={{ fontSize: 10, color: 'var(--text-3)', fontVariantNumeric: 'tabular-nums', flexShrink: 0 }}>
              {durationMs < 1000 ? `${durationMs}ms` : `${(durationMs / 1000).toFixed(1)}s`}
            </span>
          )}
        </div>

        {expanded && hasDetail && (
          <div className="animate-fade-in" style={{
            marginTop: 4, padding: '6px 10px',
            background: 'var(--bg-surface)', borderRadius: 'var(--r-md)',
            fontSize: 11, color: 'var(--text-2)',
          }}>
            {event.path && <div className="mono" style={{ marginBottom: 4 }}>{event.path}</div>}
            {event.error && <div style={{ color: 'var(--red)', marginBottom: 4 }}>{event.error}</div>}
            {event.data && Object.keys(event.data).length > 0 && (
              <div className="mono" style={{ whiteSpace: 'pre-wrap', wordBreak: 'break-word', maxHeight: 200, overflowY: 'auto' }}>
                {JSON.stringify(event.data, null, 2)}
              </div>
            )}
          </div>
        )}
      </div>
    </div>
  )
}

/* ── Main component ──────────────────────────────── */

export default function RoleTranscript({
  teamId, threadId, roleName, runId,
  model, tokens, cost, status,
}: RoleTranscriptProps) {
  const { setFocusedRole } = useStore()
  const activityQuery = useActivity({ threadId, teamId, runId, includeChildRuns: false, limit: 300 })
  const thinkingQuery = useThinkingEvents(runId)
  const activities = activityQuery.data ?? []
  const thinking = thinkingEventsToEntries(thinkingQuery.data ?? [])
  const bottomRef = useRef<HTMLDivElement>(null)

  // Merge activities and thinking into a timeline
  const timeline = useMemo(() => {
    type TimelineEntry = { type: 'activity'; event: ActivityEvent; time: number } | { type: 'thinking'; entry: ThinkingEntry; time: number }

    const items: TimelineEntry[] = [
      ...activities.map(e => ({
        type: 'activity' as const,
        event: e,
        time: e.startedAt ? new Date(e.startedAt).getTime() : 0,
      })),
      ...thinking.map(t => ({
        type: 'thinking' as const,
        entry: t,
        time: t.createdAt,
      })),
    ]
    items.sort((a, b) => a.time - b.time)
    return items
  }, [activities, thinking])

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: 'smooth' })
  }, [timeline.length])

  const isActive = status === 'running' || status === 'working' || status === 'thinking'

  return (
    <div style={{ display: 'flex', flexDirection: 'column', height: '100%' }}>
      {/* Header */}
      <div style={{
        padding: '12px 24px', borderBottom: '1px solid var(--border)',
        display: 'flex', alignItems: 'center', gap: 10,
        flexShrink: 0, background: 'var(--bg-panel)',
      }}>
        <button
          className="btn-ghost"
          onClick={() => setFocusedRole(null)}
          style={{ gap: 4, padding: '4px 8px', fontSize: 12, color: 'var(--text-2)' }}
        >
          <ChevronLeft size={14} />
          Back
        </button>

        <div style={{ width: 1, height: 16, background: 'var(--border)' }} />

        {/* Role badge */}
        <span style={{
          fontWeight: 700, fontSize: 13, color: 'var(--text-1)',
          textTransform: 'uppercase', letterSpacing: '0.04em',
        }}>
          {roleName}
        </span>

        {isActive && (
          <span className="spinner" style={{ width: 10, height: 10, borderWidth: 1.5 }} />
        )}

        <div style={{ flex: 1 }} />

        {/* Stats */}
        <div style={{ display: 'flex', gap: 14, fontSize: 11, color: 'var(--text-3)', fontVariantNumeric: 'tabular-nums' }}>
          {model && (
            <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <Cpu size={10} /> {model}
            </span>
          )}
          {(tokens ?? 0) > 0 && (
            <span style={{ display: 'flex', alignItems: 'center', gap: 4 }}>
              <Activity size={10} /> {tokens!.toLocaleString()} tok
            </span>
          )}
          {(cost ?? 0) > 0 && (
            <span style={{
              display: 'flex', alignItems: 'center', gap: 4,
              color: 'var(--amber)', fontWeight: 600,
              background: 'var(--amber-dim)',
              padding: '2px 8px', borderRadius: 999,
            }}>
              <Coins size={10} /> ${cost!.toFixed(4)}
            </span>
          )}
        </div>
      </div>

      {/* Timeline */}
      <div style={{ flex: 1, minHeight: 0, overflowY: 'auto', padding: '16px 24px' }}>
        {activityQuery.isLoading ? (
          <div style={{ display: 'flex', justifyContent: 'center', padding: 40 }}>
            <span className="spinner spinner-md" />
          </div>
        ) : timeline.length === 0 ? (
          <div style={{ textAlign: 'center', padding: 40, color: 'var(--text-3)', fontSize: 13 }}>
            No activity recorded for this role yet
          </div>
        ) : (
          timeline.map((item, i) => {
            if (item.type === 'thinking') {
              return <MiniThought key={item.entry.id} entry={item.entry} />
            }
            return <ActivityRow key={item.event.id || i} event={item.event} />
          })
        )}
        <div ref={bottomRef} />
      </div>
    </div>
  )
}
