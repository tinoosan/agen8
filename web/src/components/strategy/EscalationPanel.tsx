import type { CSSProperties } from 'react'
import { X } from 'lucide-react'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { EscalationNodeData } from './EscalationNode'
import type { OperatorUrgency, EscalationStatus } from '../../lib/types'
import { useStrategySpaceLabel } from './useStrategySpaceLabel'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { useProjectKRs } from '../../hooks/useMissions'
import { RelatedSection } from './RelatedSection'
import type { NodePanelProps } from './types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'

const STATUS_DOT: Record<EscalationStatus, string> = {
  pending: 'var(--amber)',
  resolved: 'var(--green)',
  expired: 'var(--text-3)',
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

function formatTimestamp(iso: string): string {
  if (!iso) return ''
  return iso.slice(0, 16).replace('T', ' ')
}

export function EscalationPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as EscalationNodeData
  const { escalation } = d
  const { resolveSpaceLabel } = useStrategySpaceLabel(projectId)
  const spaceLabel = resolveSpaceLabel({
    spaceLabel: (escalation as { spaceLabel?: string }).spaceLabel,
    spaceId: escalation.spaceId,
  })
  const urgency = (escalation.urgency as OperatorUrgency) ?? 'low'

  // Fetch titles for related entities
  const spacesQuery = useProjectSpaces(projectId, { refetchInterval: false })
  const tasksQuery = useProjectTasks(spacesQuery.data ?? [])
  const taskTitle = escalation.taskRef
    ? (tasksQuery.data ?? []).find(t => t.id === escalation.taskRef)?.title ?? null
    : null
  const krsQuery = useProjectKRs(projectId)
  const krTitle = escalation.keyResultRef
    ? krsQuery.data?.get(escalation.keyResultRef)?.title ?? null
    : null

  const relatedItems = [
    ...(escalation.taskRef && taskTitle ? [{ nodeId: `task:${escalation.taskRef}`, type: 'Task', title: taskTitle }] : []),
    ...(escalation.keyResultRef && krTitle ? [{ nodeId: escalation.keyResultRef, type: 'Key Result', title: krTitle }] : []),
  ]
  const status = escalation.status as EscalationStatus
  const hasConfidence = escalation.confidence != null && escalation.confidence > 0
  const confidencePercent = hasConfidence
    ? Math.round((escalation.confidence ?? 0) * 100)
    : 0
  const formattedCreated = formatTimestamp(escalation.createdAt)
  const formattedResolved = escalation.resolvedAt ? formatTimestamp(escalation.resolvedAt) : ''

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
            Escalation
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
            {escalation.title}
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
        {/* Status + urgency row */}
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
              {status}
              {escalation.category && (
                <span style={{ color: 'var(--text-3)', fontWeight: 400 }}>
                  {' · '}
                  {escalation.category}
                </span>
              )}
              {hasConfidence && (
                <span style={{ color: 'var(--text-3)', fontWeight: 400 }}>
                  {' · '}
                  {confidencePercent}% confident
                </span>
              )}
            </span>
          </span>
          <span
            className="uppercase"
            style={{
              fontFamily: SF_TEXT,
              fontSize: '10px',
              fontWeight: 600,
              letterSpacing: '0.08em',
              lineHeight: 1.33,
              color: URGENCY_COLOR[urgency] ?? 'var(--text-3)',
              whiteSpace: 'nowrap',
            }}
          >
            {urgency}
          </span>
        </div>

        {spaceLabel && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Space</p>
            <p style={PROSE_STYLE}>{spaceLabel}</p>
          </div>
        )}

        {/* Description */}
        {escalation.description && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Description</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {escalation.description}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {/* Recommendation */}
        {escalation.recommendation && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Recommendation</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {escalation.recommendation}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {/* Resolution — kicker label includes the resolution kind inline */}
        {(escalation.resolution || escalation.resolutionNote) && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p style={LABEL_STYLE}>
              <span className="uppercase">Resolution</span>
              {escalation.resolution && (
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
                  {escalation.resolution}
                </span>
              )}
            </p>
            {escalation.resolutionNote && (
              <div className="md-prose" style={PROSE_STYLE}>
                <ReactMarkdown remarkPlugins={[remarkGfm]}>
                  {escalation.resolutionNote}
                </ReactMarkdown>
              </div>
            )}
            {escalation.resolvedBy && (
              <p
                style={{
                  fontFamily: SF_TEXT,
                  fontSize: '11px',
                  fontWeight: 400,
                  letterSpacing: '-0.12px',
                  lineHeight: 1.33,
                  color: 'var(--text-3)',
                  margin: 0,
                }}
              >
                by {escalation.resolvedBy}
              </p>
            )}
          </div>
        )}

        <RelatedSection items={relatedItems} />
      </div>

      {/* Footer — escalated + (resolved) timestamps */}
      <div
        style={{
          padding: '12px 16px',
          background: 'var(--bg-panel)',
          borderTop: '1px solid var(--border)',
        }}
      >
        <div className="flex flex-col" style={{ gap: '8px' }}>
          <div className="flex flex-col" style={{ gap: '2px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Escalated</p>
            <p style={TIMESTAMP_STYLE}>{formattedCreated}</p>
          </div>
          {formattedResolved && (
            <div className="flex flex-col" style={{ gap: '2px' }}>
              <p className="uppercase" style={LABEL_STYLE}>Resolved</p>
              <p style={TIMESTAMP_STYLE}>{formattedResolved}</p>
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
