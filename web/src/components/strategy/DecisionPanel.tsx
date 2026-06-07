import type { CSSProperties } from 'react'
import { PanelHeader } from './PanelHeader'
import { RelatedSection } from './RelatedSection'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { DecisionNodeData } from './DecisionNode'
import type { DecisionSource } from '../../lib/types'
import { useKeyResult, useProjectKRs, useMissions } from '../../hooks/useMissions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import type { NodePanelProps } from './types'
import { decisionActorDisplay } from '../../lib/decisionDisplay'
import { entityDisplayTitle } from '../../lib/displaySanitizers'
import { PANEL_FONT } from './panelTypography'

const SOURCE_DOT: Record<DecisionSource, string> = {
  agent: 'var(--accent)',
}

const LABEL_STYLE: CSSProperties = {
  fontSize: '0.625rem',
  fontWeight: 500,
  letterSpacing: '0.08em',
  lineHeight: 1.33,
  color: 'var(--text-3)',
  margin: 0,
}

const PROSE_STYLE: CSSProperties = {
  fontFamily: PANEL_FONT,
  fontSize: '0.8125rem',
  fontWeight: 400,
  letterSpacing: '-0.224px',
  lineHeight: 1.43,
  color: 'var(--text-2)',
  margin: 0,
}

function formatTimestamp(iso: string): string {
  // Returns "2026-04-10 14:32" — locale-stable ISO short form
  if (!iso) return ''
  return iso.slice(0, 16).replace('T', ' ')
}

export function DecisionPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as DecisionNodeData
  const { decision } = d

  // Fetch titles for related entities
  const tasksQuery = useProjectTasks(projectId)
  const taskTitle = decision.taskRef
    ? entityDisplayTitle(decision.taskRef, (tasksQuery.data ?? []).find(t => t.id === decision.taskRef)?.title)
    : null
  const krsQuery = useProjectKRs(projectId)
  const directKrQuery = useKeyResult(decision.keyResultRef ?? null)
  const kr = decision.keyResultRef
    ? directKrQuery.data ?? krsQuery.data?.get(decision.keyResultRef)
    : undefined
  const krTitle = decision.keyResultRef
    ? kr?.title ?? null
    : null
  const missionsQuery = useMissions(projectId)
  const missionTitle = decision.missionRef
    ? entityDisplayTitle(decision.missionRef, (missionsQuery.data ?? []).find(m => m.id === decision.missionRef)?.title)
    : null

  const confidencePercent = Math.round((decision.confidence ?? 0) * 100)
  const hasConfidence = decision.confidence > 0
  const actor = decisionActorDisplay(decision)

  return (
    <div className="flex flex-col h-full">
      {/* Header — dark section, mirrors MissionPanel / KRPanel */}
      <PanelHeader onClose={onClose}>
          <p
            className="uppercase mb-1"
            style={{
              fontSize: '0.625rem',
              fontWeight: 500,
              letterSpacing: '0.08em',
              lineHeight: 1.33,
              color: 'var(--text-3)',
            }}
          >
            Decision
          </p>
          <h2
            className="text-foreground line-clamp-3"
            style={{
              fontFamily: PANEL_FONT,
              fontSize: '1.0625rem',
              fontWeight: 600,
              lineHeight: 1.24,
              letterSpacing: '-0.374px',
            }}
          >
            {decision.title}
          </h2>
      </PanelHeader>

      {/* Body — scrollable content sections */}
      <div
        className="flex-1 overflow-y-auto flex flex-col"
        style={{
          background: 'var(--bg-panel)',
          padding: '16px',
          gap: '16px',
          fontFamily: PANEL_FONT,
        }}
      >
        {/* Source + confidence — typographic metadata, not a progress indicator */}
        <div className="flex items-center justify-between" style={{ gap: '8px' }}>
          <span
            className="flex items-center min-w-0"
            style={{
              fontFamily: PANEL_FONT,
              fontSize: '0.75rem',
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
                background: SOURCE_DOT[decision.source] ?? 'var(--text-3)',
                display: 'inline-block',
                flexShrink: 0,
              }}
            />
            <span className="truncate">
              {decision.source}
              {actor.label && actor.label !== decision.source && (
                <span style={{ color: 'var(--text-3)', fontWeight: 400 }}>
                  {' · '}
                  {actor.label}
                </span>
              )}
            </span>
          </span>
          {hasConfidence && (
            <span
              style={{
                fontFamily: PANEL_FONT,
                fontSize: '0.6875rem',
                fontWeight: 400,
                letterSpacing: '-0.12px',
                lineHeight: 1.33,
                color: 'var(--text-3)',
                whiteSpace: 'nowrap',
                fontVariantNumeric: 'tabular-nums',
              }}
            >
              {confidencePercent}% confidence
            </span>
          )}
        </div>

        {/* Rationale */}
        {decision.rationale && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Rationale</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {decision.rationale}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {/* Alternatives rejected */}
        {decision.alternativesRejected && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Alternatives rejected</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {decision.alternativesRejected}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {decision.invalidationConditions && decision.invalidationConditions.length > 0 && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Invalidation conditions</p>
            <ul style={{ ...PROSE_STYLE, paddingLeft: 18 }}>
              {decision.invalidationConditions.map((condition) => (
                <li key={condition}>{condition}</li>
              ))}
            </ul>
          </div>
        )}

        {/* Outcome */}
        {decision.outcome && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Outcome</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {decision.outcome}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {/* Tags */}
        {decision.tags && decision.tags.length > 0 && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Tags</p>
            <div className="flex flex-wrap" style={{ gap: '4px' }}>
              {decision.tags.map((tag) => (
                <span
                  key={tag}
                  style={{
                    display: 'inline-block',
                    padding: '2px 8px',
                    borderRadius: '4px',
                    background: 'var(--bg-elevated)',
                    fontFamily: PANEL_FONT,
                    fontSize: '0.625rem',
                    fontWeight: 500,
                    letterSpacing: '-0.08px',
                    lineHeight: 1.47,
                    color: 'var(--text-2)',
                  }}
                >
                  {tag}
                </span>
              ))}
            </div>
          </div>
        )}

        {/* Related nodes — backlinks to parent entities */}
        <RelatedSection items={[
          ...(decision.taskRef && taskTitle ? [{ nodeId: `task:${decision.taskRef}`, type: 'Task', title: taskTitle }] : []),
          ...(decision.keyResultRef ? [{ nodeId: decision.keyResultRef, type: 'Key Result', title: krTitle ?? 'Key result' }] : []),
          ...(decision.missionRef && missionTitle ? [{ nodeId: decision.missionRef, type: 'Mission', title: missionTitle }] : []),
        ]} />
      </div>

      {/* Footer — created timestamp */}
      <div
        style={{
          padding: '12px 16px',
          background: 'var(--bg-panel)',
          borderTop: '1px solid var(--border)',
        }}
      >
        <p
          className="uppercase"
          style={{
            ...LABEL_STYLE,
            marginBottom: '4px',
          }}
        >
          Created
        </p>
        <p
          style={{
            fontFamily: PANEL_FONT,
            fontSize: '0.75rem',
            fontWeight: 400,
            letterSpacing: '-0.12px',
            lineHeight: 1.33,
            color: 'var(--text-2)',
            margin: 0,
            fontVariantNumeric: 'tabular-nums',
          }}
        >
          {formatTimestamp(decision.createdAt)}
        </p>
      </div>
    </div>
  )
}
