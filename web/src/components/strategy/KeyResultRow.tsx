import { ChevronRight, ExternalLink } from 'lucide-react'
import KRDetailBody from './KRDetailBody'
import { NodeLink } from './NodeLink'
import type { KeyResultView, KeyResultStatus } from '../../lib/types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'

const KR_STATUS_DOT: Record<KeyResultStatus, string> = {
  open: 'var(--text-3)',
  on_track: 'var(--green)',
  at_risk: 'var(--amber)',
  completed: 'var(--accent)',
  dropped: 'var(--red)',
}

interface KeyResultRowProps {
  kr: KeyResultView
  expanded: boolean
  onToggle: () => void
  isLast?: boolean
}

/**
 * KR row used inside MissionPanel's KR list.
 *
 * Collapsed: title, progress bar, status dot, percentage. Click anywhere on
 * the header to expand.
 *
 * Expanded: wraps the shared `<KRDetailBody>` in an elevated container so it
 * visually "lifts" out of the surrounding KR list.
 */
export default function KeyResultRow({ kr, expanded, onToggle, isLast }: KeyResultRowProps) {
  return (
    <div
      className="flex flex-col"
      style={{
        paddingTop: '12px',
        paddingBottom: '12px',
        borderBottom: isLast ? 'none' : '1px solid var(--border)',
        fontFamily: SF_TEXT,
      }}
    >
      {/* Header — click or keyboard to toggle expansion */}
      <div
        role="button"
        tabIndex={0}
        onClick={onToggle}
        onKeyDown={(e) => {
          if (e.key === 'Enter' || e.key === ' ') {
            e.preventDefault()
            onToggle()
          }
        }}
        className="flex flex-col cursor-pointer select-none"
        style={{ gap: '6px' }}
        aria-expanded={expanded}
      >
        <div className="flex items-center" style={{ gap: '6px' }}>
          <ChevronRight
            size={10}
            className="shrink-0 transition-transform duration-150"
            style={{
              color: 'var(--text-3)',
              transform: expanded ? 'rotate(90deg)' : 'rotate(0deg)',
            }}
          />
          <p
            className="line-clamp-1 flex-1"
            style={{
              fontSize: '0.875rem',
              fontWeight: 400,
              lineHeight: 1.29,
              letterSpacing: '-0.224px',
              color: 'var(--text-1)',
            }}
          >
            {kr.title}
          </p>
        </div>
        <div
          style={{
            height: '2px',
            borderRadius: '980px',
            background: 'var(--bg-elevated)',
            overflow: 'hidden',
            marginLeft: '16px',
          }}
        >
          <div
            style={{
              height: '100%',
              borderRadius: '980px',
              width: `${Math.min(100, Math.max(0, kr.progressPercent))}%`,
              background: 'var(--accent)',
              transition: 'width 0.3s ease',
            }}
          />
        </div>
        <div
          className="flex justify-between items-center"
          style={{ marginLeft: '16px' }}
        >
          <span
            className="flex items-center"
            style={{
              fontSize: '0.625rem',
              fontWeight: 400,
              letterSpacing: '-0.08px',
              lineHeight: 1.47,
              color: 'var(--text-3)',
              gap: '4px',
            }}
          >
            <span
              style={{
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: KR_STATUS_DOT[kr.status],
                display: 'inline-block',
              }}
            />
            {kr.status.replace('_', '\u00A0')}
          </span>
          <span
            style={{
              fontSize: '0.625rem',
              fontWeight: 600,
              letterSpacing: '-0.08px',
              lineHeight: 1.47,
              color: 'var(--text-2)',
            }}
          >
            {kr.progressPercent}%
          </span>
        </div>
      </div>

      {/* Expanded — elevated container wrapping the shared detail body */}
      {expanded && (
        <div
          className="flex flex-col"
          style={{
            gap: '14px',
            marginTop: '14px',
            marginLeft: '16px',
            padding: '14px',
            borderRadius: '12px',
            background: 'var(--bg-elevated)',
            fontFamily: SF_TEXT,
          }}
        >
          <KRDetailBody kr={kr} />
          <NodeLink nodeId={kr.id} className="inline-flex items-center gap-1.5 self-start hover:opacity-80 transition-opacity">
            <ExternalLink size={10} style={{ color: 'var(--accent)' }} />
            <span style={{ fontSize: '0.6875rem', fontWeight: 500, color: 'var(--accent)' }}>
              Go to node
            </span>
          </NodeLink>
        </div>
      )}
    </div>
  )
}
