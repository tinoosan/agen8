import type { CSSProperties } from 'react'
import { X } from 'lucide-react'
import { RelatedSection } from './RelatedSection'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import type { DecisionNodeData } from './DecisionNode'
import type { DecisionSource } from '../../lib/types'
import { useKeyResult, useProjectKRs, useMissions } from '../../hooks/useMissions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import type { NodePanelProps } from './types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'

const SOURCE_DOT: Record<DecisionSource, string> = {
  agent: 'var(--accent)',
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

function formatTimestamp(iso: string): string {
  // Returns "2026-04-10 14:32" — locale-stable ISO short form
  if (!iso) return ''
  return iso.slice(0, 16).replace('T', ' ')
}

function answerText(answer: { selectedOption?: string; selectedOptions?: string[]; freeFormText?: string } | undefined): string {
  if (!answer) return 'No answer recorded'
  const multi = (answer.selectedOptions ?? []).map((v) => v.trim()).filter((v) => v !== '')
  const selected = answer.selectedOption?.trim() ?? ''
  const freeForm = answer.freeFormText?.trim() ?? ''
  // multi_select takes precedence — if SelectedOptions is populated
  // the answer is fundamentally a list, even when a free-form
  // fallback was also provided.
  if (multi.length > 0) {
    return freeForm && !multi.includes(freeForm)
      ? `${multi.join(', ')}\n${freeForm}`
      : multi.join(', ')
  }
  if (selected && freeForm && freeForm !== selected) {
    return `${selected}\n${freeForm}`
  }
  if (selected) return selected
  if (freeForm) return freeForm
  return 'No answer recorded'
}

export function DecisionPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as DecisionNodeData
  const { decision } = d
  const isAskUser = (decision.kind ?? '').trim().toLowerCase() === 'ask_user'

  // Fetch titles for related entities
  const tasksQuery = useProjectTasks(projectId)
  const taskTitle = decision.taskRef
    ? (tasksQuery.data ?? []).find(t => t.id === decision.taskRef)?.title ?? decision.taskRef.slice(0, 12)
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
    ? (missionsQuery.data ?? []).find(m => m.id === decision.missionRef)?.title ?? decision.missionRef.slice(0, 12)
    : null

  const confidencePercent = Math.round((decision.confidence ?? 0) * 100)
  const hasConfidence = decision.confidence > 0

  return (
    <div className="flex flex-col h-full">
      {/* Header — dark section, mirrors MissionPanel / KRPanel */}
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
            Decision
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
            {decision.title}
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

      {/* Body — scrollable content sections */}
      <div
        className="flex-1 overflow-y-auto flex flex-col"
        style={{
          background: 'var(--bg-panel)',
          padding: '16px',
          gap: '16px',
          fontFamily: SF_TEXT,
        }}
      >
        {/* Source + confidence — typographic metadata, not a progress indicator */}
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
                background: SOURCE_DOT[decision.source] ?? 'var(--text-3)',
                display: 'inline-block',
                flexShrink: 0,
              }}
            />
            <span className="truncate">
              {decision.source}
              {(decision.memberName?.trim() || decision.sourceIdentity) && (
                <span style={{ color: 'var(--text-3)', fontWeight: 400 }}>
                  {' · '}
                  {decision.memberName?.trim() || decision.sourceIdentity}
                </span>
              )}
            </span>
          </span>
          {hasConfidence && (
            <span
              style={{
                fontFamily: SF_TEXT,
                fontSize: '11px',
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

        {isAskUser && decision.context && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Context</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {decision.context}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {isAskUser && decision.questions && decision.questions.length > 0 && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Questions</p>
            <div className="flex flex-col" style={{ gap: '12px' }}>
              {decision.questions.map((question, index) => {
                const answer = decision.answers?.find((entry) => entry.questionId === question.id)
                return (
                  <div key={question.id || `${index}-${question.text}`} className="flex flex-col" style={{ gap: '4px' }}>
                    <p
                      style={{
                        fontFamily: SF_TEXT,
                        fontSize: '13px',
                        fontWeight: 600,
                        letterSpacing: '-0.18px',
                        lineHeight: 1.38,
                        color: 'var(--text-1)',
                        margin: 0,
                      }}
                    >
                      {question.text}
                      {question.blocking && (
                        <span
                          style={{
                            display: 'inline-block',
                            marginLeft: 8,
                            padding: '1px 7px',
                            borderRadius: 999,
                            background: 'color-mix(in srgb, var(--amber) 16%, transparent)',
                            color: 'var(--amber)',
                            fontSize: 9,
                            fontWeight: 700,
                            letterSpacing: '0.04em',
                            textTransform: 'uppercase',
                          }}
                        >
                          Blocking
                        </span>
                      )}
                    </p>
                    <p style={{ ...PROSE_STYLE, whiteSpace: 'pre-wrap' }}>{answerText(answer)}</p>
                    {question.recommendation && (
                      <p
                        style={{
                          fontFamily: SF_TEXT,
                          fontSize: '11px',
                          fontWeight: 500,
                          letterSpacing: '-0.08px',
                          lineHeight: 1.4,
                          color: 'var(--text-3)',
                          margin: 0,
                        }}
                      >
                        Recommendation: {question.recommendation}
                      </p>
                    )}
                  </div>
                )
              })}
            </div>
          </div>
        )}

        {isAskUser && decision.cancelled && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Status</p>
            <p style={PROSE_STYLE}>Cancelled.</p>
          </div>
        )}

        {/* Rationale */}
        {!isAskUser && decision.rationale && (
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
        {!isAskUser && decision.alternativesRejected && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p className="uppercase" style={LABEL_STYLE}>Alternatives rejected</p>
            <div className="md-prose" style={PROSE_STYLE}>
              <ReactMarkdown remarkPlugins={[remarkGfm]}>
                {decision.alternativesRejected}
              </ReactMarkdown>
            </div>
          </div>
        )}

        {!isAskUser && decision.invalidationConditions && decision.invalidationConditions.length > 0 && (
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
        {!isAskUser && decision.outcome && (
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
                    fontFamily: SF_TEXT,
                    fontSize: '10px',
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
            fontFamily: SF_TEXT,
            fontSize: '12px',
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
