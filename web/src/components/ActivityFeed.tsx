import { useRef, useState } from 'react'
import { useActivity } from '../hooks/useActivity'
import type { ActivityEvent } from '../lib/types'
import { ChevronDown, ChevronRight } from 'lucide-react'

interface ActivityFeedProps {
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
  const typeColor = event.type ? getTypeColor(event.type) : 'var(--text-3)'

  return (
    <div
      style={{
        padding: '5px 0',
        borderBottom: '1px solid var(--border)',
        cursor: event.detail ? 'pointer' : 'default',
      }}
      onClick={() => event.detail && setExpanded(e => !e)}
    >
      <div style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
        {event.detail && (
          <span style={{ color: 'var(--text-3)', flexShrink: 0, display: 'flex', alignItems: 'center' }}>
            {expanded ? <ChevronDown size={10} /> : <ChevronRight size={10} />}
          </span>
        )}
        {!event.detail && <span style={{ width: 10, flexShrink: 0 }} />}
        {event.role && (
          <span style={{ fontSize: 10, fontWeight: 600, color: 'var(--text-2)', flexShrink: 0 }}>
            {event.role}
          </span>
        )}
        {event.type && (
          <span style={{ fontSize: 9, color: typeColor, fontFamily: 'inherit', flexShrink: 0, fontWeight: 500, letterSpacing: '0.03em', textTransform: 'uppercase' }}>
            {event.type}
          </span>
        )}
        <span style={{ fontSize: 11, color: 'var(--text-2)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
          {event.summary}
        </span>
      </div>
      {expanded && event.detail && (
        <div style={{
          marginTop: 5, marginLeft: 16, padding: '6px 8px',
          background: 'var(--bg-elevated)',
          borderRadius: 'var(--r-md)',
          border: '1px solid var(--border)',
          fontSize: 10, color: 'var(--text-2)',
          whiteSpace: 'pre-wrap', wordBreak: 'break-all',
          maxHeight: 180, overflow: 'auto',
          lineHeight: 1.6,
        }} className="mono">
          {event.detail}
        </div>
      )}
    </div>
  )
}

export default function ActivityFeed({ teamId }: ActivityFeedProps) {
  const query = useActivity(teamId)
  const containerRef = useRef<HTMLDivElement>(null)
  const events = query.data ?? []
  const recent = events.slice(-50)

  return (
    <div
      ref={containerRef}
      style={{ overflowY: 'auto', flex: 1, minHeight: 0 }}
    >
      {recent.length === 0 ? (
        <div style={{ fontSize: 11, color: 'var(--text-3)', padding: '10px 0' }}>No activity yet</div>
      ) : (
        recent.map((event, i) => <EventRow key={event.seq ?? i} event={event} />)
      )}
    </div>
  )
}
