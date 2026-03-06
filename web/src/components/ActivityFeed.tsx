import { useRef, useState } from 'react'
import { useActivity } from '../hooks/useActivity'
import type { ActivityEvent } from '../lib/types'

interface ActivityFeedProps {
  teamId: string
}

function EventRow({ event }: { event: ActivityEvent }) {
  const [expanded, setExpanded] = useState(false)

  return (
    <div
      style={{ padding: '4px 0', borderBottom: '1px solid light-dark(rgba(0,0,0,0.05), rgba(255,255,255,0.05))', cursor: event.detail ? 'pointer' : 'default' }}
      onClick={() => event.detail && setExpanded(e => !e)}
    >
      <div style={{ display: 'flex', gap: 6, alignItems: 'baseline' }}>
        {event.role && (
          <span style={{ fontSize: 10, fontWeight: 700, opacity: 0.5, flexShrink: 0 }}>
            {event.role}
          </span>
        )}
        {event.type && (
          <span style={{ fontSize: 10, opacity: 0.4, fontFamily: 'monospace', flexShrink: 0 }}>
            {event.type}
          </span>
        )}
        <span style={{ fontSize: 11, opacity: 0.7, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1 }}>
          {event.summary}
        </span>
      </div>
      {expanded && event.detail && (
        <div style={{
          marginTop: 4, padding: '6px 8px',
          background: 'light-dark(rgba(0,0,0,0.04), rgba(255,255,255,0.04))',
          borderRadius: 6,
          fontSize: 10, fontFamily: 'monospace', opacity: 0.7,
          whiteSpace: 'pre-wrap', wordBreak: 'break-all',
          maxHeight: 200, overflow: 'auto',
        }}>
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
        <div style={{ fontSize: 11, opacity: 0.35, padding: '8px 0' }}>No activity yet</div>
      ) : (
        recent.map((event, i) => <EventRow key={event.seq ?? i} event={event} />)
      )}
    </div>
  )
}
