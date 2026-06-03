import type { CSSProperties } from 'react'
import { X } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { OANodeData } from './OANode'
import type { OperatorUrgency, OpActionStatus } from '../../lib/types'
import { useStrategySpaceLabel } from './useStrategySpaceLabel'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { useProjectKRs } from '../../hooks/useMissions'
import { RelatedSection } from './RelatedSection'
import type { NodePanelProps } from './types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'

const STATUS_DOT: Record<OpActionStatus, string> = {
  pending: 'var(--text-3)',
  acknowledged: 'var(--amber)',
  in_progress: 'var(--amber)',
  pending_verification: 'var(--amber)',
  completed: 'var(--green)',
  blocked: 'var(--red)',
  canceled: 'var(--text-3)',
}

const URGENCY_COLOR: Record<OperatorUrgency, string> = {
  low: 'var(--text-3)',
  medium: 'var(--text-2)',
  high: 'var(--amber)',
  critical: 'var(--red)',
}

const LABEL_STYLE: CSSProperties = {
  fontSize: '10px',
  fontWeight: 500,
  letterSpacing: '0.08em',
  lineHeight: 1.33,
  color: 'var(--text-3)',
  margin: 0,
}

const PROSE_STYLE: CSSProperties = {
  fontFamily: SF_TEXT,
  fontSize: '13px',
  fontWeight: 400,
  letterSpacing: '-0.224px',
  lineHeight: 1.43,
  color: 'var(--text-2)',
  margin: 0,
}

const TIMESTAMP_STYLE: CSSProperties = {
  fontFamily: SF_TEXT,
  fontSize: '12px',
  fontWeight: 400,
  letterSpacing: '-0.12px',
  lineHeight: 1.33,
  color: 'var(--text-2)',
  margin: 0,
  fontVariantNumeric: 'tabular-nums',
}

const RIGHT_CHIP_STYLE: CSSProperties = {
  fontFamily: SF_TEXT,
  fontSize: '10px',
  fontWeight: 600,
  letterSpacing: '0.08em',
  lineHeight: 1.33,
  whiteSpace: 'nowrap',
  textTransform: 'uppercase',
}

function formatTimestamp(iso: string): string {
  if (!iso) return ''
  return iso.slice(0, 16).replace('T', ' ')
}

export function OAPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as OANodeData
  const { oa } = d
  const { resolveSpaceLabel } = useStrategySpaceLabel(projectId)
  const spaceLabel = resolveSpaceLabel({ spaceLabel: (oa as { spaceLabel?: string }).spaceLabel, spaceId: oa.spaceId })
  const urgency = (oa.urgency as OperatorUrgency) ?? 'low'
  const status = oa.status as OpActionStatus
  const formattedCreated = formatTimestamp(oa.createdAt)
  const formattedCompleted = oa.completedAt ? formatTimestamp(oa.completedAt) : ''

  // Fetch titles for related entities
  const spacesQuery = useProjectSpaces(projectId, { refetchInterval: false })
  const tasksQuery = useProjectTasks(spacesQuery.data ?? [])
  const taskTitle = oa.taskRef
    ? (tasksQuery.data ?? []).find(t => t.id === oa.taskRef)?.title ?? null
    : null
  const krsQuery = useProjectKRs(projectId)
  const krTitle = oa.keyResultRef
    ? krsQuery.data?.get(oa.keyResultRef)?.title ?? null
    : null

  const relatedItems = [
    ...(oa.taskRef && taskTitle ? [{ nodeId: `task:${oa.taskRef}`, type: 'Task', title: taskTitle }] : []),
    ...(oa.keyResultRef && krTitle ? [{ nodeId: oa.keyResultRef, type: 'Key Result', title: krTitle }] : []),
  ]

  return (
    <div className="flex flex-col h-full">
      {/* Header — dark section */}
      <div
        className="flex items-start gap-2 shrink-0"
        style={{ background: 'var(--bg-app)', padding: '16px' }}
      >
        <div className="flex-1 min-w-0">
          <p
            className="uppercase mb-1"
            style={{
              fontSize: '10px',
              fontWeight: 500,
              letterSpacing: '0.08em',
              lineHeight: 1.33,
              color: 'var(--text-3)',
            }}
          >
            Operator Action
          </p>
          <h2
            className="text-foreground line-clamp-3"
            style={{
              fontFamily: SF_TEXT,
              fontSize: '17px',
              fontWeight: 600,
              lineHeight: 1.24,
              letterSpacing: '-0.374px',
            }}
          >
            {oa.title}
          </h2>
        </div>
        <button
          onClick={onClose}
          className="shrink-0 text-muted-foreground hover:text-foreground transition-colors mt-0.5"
          style={{ padding: '4px', borderRadius: '50%', background: 'rgba(255,255,255,0.08)' }}
          aria-label="Close panel"
        >
          <X size={14} />
        </button>
      </div>

      {/* Body */}
      <div
        className="flex-1 overflow-y-auto flex flex-col"
        style={{
          background: 'var(--bg-panel)',
          padding: '16px',
          gap: '16px',
          fontFamily: SF_TEXT,
        }}
      >
        {/* Status + (blocking) + urgency row */}
        <div className="flex items-center justify-between" style={{ gap: '8px' }}>
          <span
            className="flex items-center min-w-0"
            style={{
              fontFamily: SF_TEXT,
              fontSize: '12px',
              fontWeight: 600,
              letterSpacing: '-0.12px',
              lineHeight: 1.33,
              color: 'var(--text-2)',
              gap: '6px',
            }}
          >
            <span
              style={{
                width: 6,
                height: 6,
                borderRadius: '50%',
                background: STATUS_DOT[status] ?? 'var(--text-3)',
                display: 'inline-block',
                flexShrink: 0,
              }}
            />
            <span className="truncate">
              {status.replace(/_/g, '\u00A0')}
              {oa.category && (
                <span style={{ color: 'var(--text-3)', fontWeight: 400 }}>
                  {' · '}
                  {oa.category}
                </span>
              )}
            </span>
          </span>
          <span className="flex items-center" style={{ gap: '8px' }}>
            {oa.blocking && (
              <span style={{ ...RIGHT_CHIP_STYLE, color: 'var(--red)' }}>
                blocking
              </span>
            )}
            <span style={{ ...RIGHT_CHIP_STYLE, color: URGENCY_COLOR[urgency] ?? 'var(--text-3)' }}>
              {urgency}
            </span>
          </span>
        </div>

        {spaceLabel && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Space</p>
            <p style={PROSE_STYLE}>{spaceLabel}</p>
          </div>
        )}

        {/* Description */}
        {oa.description && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Description</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {oa.description}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {/* Outcome — kicker includes outcomeStatus inline */}
        {oa.outcomeSummary && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p style={LABEL_STYLE}>
              <span className="uppercase">Outcome</span>
              {oa.outcomeStatus && (
                <span
                  style={{
                    color: 'var(--text-2)',
                    textTransform: 'none',
                    letterSpacing: '-0.12px',
                    fontWeight: 600,
                    fontSize: '11px',
                  }}
                >
                  {' · '}
                  {oa.outcomeStatus}
                </span>
              )}
            </p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {oa.outcomeSummary}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {/* Progress notes */}
        {oa.progressNotes && oa.progressNotes.length > 0 && (
          <div className="flex flex-col" style={{ gap: '8px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Progress notes</p>
            <div className="flex flex-col" style={{ gap: '12px' }}>
              {oa.progressNotes.slice(0, 5).map((note, i) => (
                <div
                  key={`${note.createdAt}-${i}`}
                  className="flex flex-col"
                  style={{ gap: '2px' }}
                >
                  <p style={PROSE_STYLE}>{note.text}</p>
                  <p
                    style={{
                      fontFamily: SF_TEXT,
                      fontSize: '10px',
                      fontWeight: 400,
                      letterSpacing: '-0.08px',
                      lineHeight: 1.33,
                      color: 'var(--text-3)',
                      margin: 0,
                      fontVariantNumeric: 'tabular-nums',
                    }}
                  >
                    {formatTimestamp(note.createdAt)}
                  </p>
                </div>
              ))}
            </div>
          </div>
        )}

        <RelatedSection items={relatedItems} />
      </div>

      {/* Footer — created + (completed) timestamps */}
      <div
        style={{
          padding: '12px 16px',
          background: 'var(--bg-panel)',
          borderTop: '1px solid var(--border)',
        }}
      >
        <div className="flex flex-col" style={{ gap: '8px' }}>
          <div className="flex flex-col" style={{ gap: '2px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Created</p>
            <p style={TIMESTAMP_STYLE}>{formattedCreated}</p>
          </div>
          {formattedCompleted && (
            <div className="flex flex-col" style={{ gap: '2px' }}>
              <p className="uppercase" style={LABEL_STYLE}>Completed</p>
              <p style={TIMESTAMP_STYLE}>{formattedCompleted}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
