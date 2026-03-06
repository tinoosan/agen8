import { useRef, useState, useEffect } from 'react'
import { useActivity } from '../hooks/useActivity'
import type { ActivityEvent } from '../lib/types'
import { ChevronRight, Activity } from 'lucide-react'

interface ActivityFeedProps {
  threadId: string | null
  teamId: string
}

const typeColors: Record<string, string> = {
  error: 'var(--red)',
  fail: 'var(--red)',
  done: 'var(--green)',
  complete: 'var(--green)',
  start: 'var(--accent)',
  tool: 'var(--amber)',
}

function getTypeColor(type: string): string {
  const lower = type.toLowerCase()
  for (const [key, color] of Object.entries(typeColors)) {
    if (lower.includes(key)) return color
  }
  return 'var(--text-3)'
}

function EventRow({ event }: { event: ActivityEvent }) {
  const [expanded, setExpanded] = useState(false)
  const typeColor = event.kind ? getTypeColor(event.kind) : 'var(--text-3)'
  const role = event.data?.role || event.data?.agent_role
  const summary = event.title || event.outputPreview || event.textPreview || event.path || event.kind
  const detail = [event.textPreview, event.outputPreview, event.error].filter(Boolean).join('\n\n')

  return (
    <div
      className="row-hover"
      style={{
        padding: '5px 4px',
        cursor: detail ? 'pointer' : 'default',
        marginBottom: 1,
      }}
      onClick={() => detail && setExpanded(e => !e)}
    >
      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        {detail ? (
          <span style={{ color: 'var(--text-3)', flexShrink: 0, display: 'flex', alignItems: 'center' }}>
            <ChevronRight
              size={10}
              style={{
                transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
                transition: 'transform 0.15s',
              }}
            />
          </span>
        ) : (
          <span style={{ width: 10, flexShrink: 0 }} />
        )}
        {role && (
          <span style={{ fontSize: 10, fontWeight: 600, color: 'var(--text-2)', flexShrink: 0 }}>
            {role}
          </span>
        )}
        {event.kind && (
          <span style={{
            fontSize: 9, color: typeColor, flexShrink: 0,
            fontWeight: 500, letterSpacing: '0.03em', textTransform: 'uppercase',
          }}>
            {event.kind}
          </span>
        )}
        <span className="truncate" style={{ fontSize: 11, color: 'var(--text-2)', flex: 1 }}>
          {summary}
        </span>
      </div>
      {expanded && detail && (
        <div className="mono" style={{
          marginTop: 5, marginLeft: 16, padding: '6px 8px',
          background: 'var(--bg-elevated)',
          borderRadius: 'var(--r-md)',
          border: '1px solid var(--border)',
          fontSize: 10, color: 'var(--text-2)',
          whiteSpace: 'pre-wrap', wordBreak: 'break-all',
          maxHeight: 180, overflow: 'auto',
          lineHeight: 1.6,
        }}>
          {detail}
        </div>
      )}
    </div>
  )
}

export default function ActivityFeed({ threadId, teamId }: ActivityFeedProps) {
  const query = useActivity({ threadId, teamId, includeChildRuns: true, limit: 100 })
  const containerRef = useRef<HTMLDivElement>(null)
  const events = query.data ?? []
  const recent = events.slice(-50)

  // Auto-scroll to latest event
  useEffect(() => {
    const el = containerRef.current
    if (el) el.scrollTop = el.scrollHeight
  }, [recent.length])

  return (
    <div
      ref={containerRef}
      style={{ overflowY: 'auto', flex: 1, minHeight: 0 }}
    >
      {recent.length === 0 ? (
        <div style={{
          display: 'flex', flexDirection: 'column', alignItems: 'center',
          padding: '20px 8px', gap: 6, textAlign: 'center',
        }}>
          <Activity size={16} style={{ color: 'var(--text-3)' }} />
          <div style={{ fontSize: 11, color: 'var(--text-3)' }}>No activity yet</div>
        </div>
      ) : (
        recent.map((event) => <EventRow key={event.id} event={event} />)
      )}
    </div>
  )
}
