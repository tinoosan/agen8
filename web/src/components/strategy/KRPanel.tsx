import { X, ExternalLink } from 'lucide-react'
import { useLocation } from 'wouter'
import { Button } from '@/components/ui/button'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useMissions } from '../../hooks/useMissions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { useStrategyMapOpActions } from '../../hooks/useOpActions'
import { useAllEscalations } from '../../hooks/useEscalations'
import { missionDetailLink } from '../../lib/routing'
import { RelatedSection } from './RelatedSection'
import { useStrategySpaceLabel } from './useStrategySpaceLabel'
import KRDetailBody from './KRDetailBody'
import type { KRNodeData } from './KRNode'
import type { KeyResultStatus } from '../../lib/types'
import type { NodePanelProps } from './types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'

const KR_STATUS_DOT: Record<KeyResultStatus, string> = {
  open: 'var(--text-3)',
  on_track: 'var(--green)',
  at_risk: 'var(--amber)',
  completed: 'var(--accent)',
  dropped: 'var(--red)',
}

export function KRPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as KRNodeData
  const { kr } = d
  const clusterColor = d.clusterColor ?? 'var(--accent)'
  const krSpaceLabel = (kr as { spaceLabel?: string }).spaceLabel
  const { resolveSpaceLabel } = useStrategySpaceLabel(projectId)
  const spaceLabel = resolveSpaceLabel({ spaceLabel: krSpaceLabel, spaceId: kr.spaceId })
  const [, navigate] = useLocation()

  const missionsQuery = useMissions(projectId)
  const missionTitle = (missionsQuery.data ?? []).find(m => m.id === kr.missionId)?.title

  // Fetch all related entities for this KR
  const decisionsQuery = useRecentDecisions(projectId)
  const linkedDecisions = (decisionsQuery.data ?? []).filter(d => d.keyResultRef === kr.id)

  const spacesQuery = useProjectSpaces(projectId, { refetchInterval: false })
  const tasksQuery = useProjectTasks(spacesQuery.data ?? [])
  const linkedTasks = (tasksQuery.data ?? []).filter(t => t.keyResultRef === kr.id)

  const oasQuery = useStrategyMapOpActions(projectId)
  const linkedOAs = (oasQuery.data ?? []).filter(oa => oa.keyResultRef === kr.id)

  const escalationsQuery = useAllEscalations(projectId)
  const linkedEscalations = (escalationsQuery.data ?? []).filter(e => e.keyResultRef === kr.id)

  const relatedItems = [
    { nodeId: kr.missionId, type: 'Mission' as const, title: missionTitle ?? kr.missionId.slice(0, 12) },
    ...linkedTasks.map(t => ({
      nodeId: `task:${t.id}`,
      type: 'Task' as const,
      title: t.title ?? t.description ?? t.id.slice(0, 12),
    })),
    ...linkedDecisions.map(dec => ({
      nodeId: `decision:${dec.id}`,
      type: 'Decision' as const,
      title: dec.title,
      ...(dec.confidence > 0 ? {
        badge: `${Math.round(dec.confidence * 100)}%`,
        badgeColor: dec.confidence >= 0.8 ? 'var(--green)' : dec.confidence >= 0.6 ? 'var(--amber)' : 'var(--red)',
      } : {}),
    })),
    ...linkedOAs.map(oa => ({
      nodeId: `oa:${oa.id}`,
      type: 'Operator Action' as const,
      title: oa.title,
    })),
    ...linkedEscalations.map(esc => ({
      nodeId: `escalation:${esc.id}`,
      type: 'Escalation' as const,
      title: esc.title,
      badge: esc.urgency,
      badgeColor: esc.urgency === 'critical' ? 'var(--red)' : esc.urgency === 'high' ? 'var(--amber)' : 'var(--text-3)',
    })),
  ]

  return (
    <div className="flex flex-col h-full">
      {/* Header — dark section, mirrors MissionPanel */}
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
            Key Result
          </p>
          <h2
            className="text-foreground line-clamp-2"
            style={{
              fontFamily: SF_TEXT,
              fontSize: '17px',
              fontWeight: 600,
              lineHeight: 1.24,
              letterSpacing: '-0.374px',
            }}
          >
            {kr.title}
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

      {/* Body — scrollable, holds the detail content + linked decisions */}
      <div
        className="flex-1 overflow-y-auto flex flex-col"
        style={{
          background: 'var(--bg-panel)',
          padding: '16px',
          gap: '16px',
          fontFamily: SF_TEXT,
        }}
      >
        {/* Status + percent — at-a-glance summary */}
        <div className="flex items-center justify-between" style={{ gap: '8px' }}>
          <span
            className="flex items-center"
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
                background: KR_STATUS_DOT[kr.status],
                display: 'inline-block',
              }}
            />
            {kr.status.replace('_', '\u00A0')}
          </span>
          <span
            style={{
              fontFamily: SF_TEXT,
              fontSize: '15px',
              fontWeight: 600,
              letterSpacing: '-0.224px',
              lineHeight: 1.24,
              color: 'var(--text-1)',
              fontVariantNumeric: 'tabular-nums',
            }}
          >
            {kr.progressPercent}%
          </span>
        </div>

        {/* Progress bar — Apple Blue, matches MissionPanel's avg-progress bar */}
        <div
          style={{
            height: '3px',
            borderRadius: '980px',
            background: 'var(--bg-elevated)',
            overflow: 'hidden',
          }}
        >
          <div
            style={{
              height: '100%',
              borderRadius: '980px',
              width: `${Math.min(100, Math.max(0, kr.progressPercent))}%`,
              background: clusterColor,
              transition: 'width 0.3s ease',
            }}
          />
        </div>

        {spaceLabel && (
          <div className="flex flex-col" style={{ gap: '6px' }}>
            <p
              className="uppercase"
              style={{
                fontSize: '10px',
                fontWeight: 500,
                letterSpacing: '0.08em',
                lineHeight: 1.33,
                color: 'var(--text-3)',
                margin: 0,
              }}
            >
              Space
            </p>
            <p
              style={{
                fontFamily: SF_TEXT,
                fontSize: '12px',
                fontWeight: 500,
                letterSpacing: '-0.12px',
                lineHeight: 1.33,
                color: 'var(--text-2)',
                margin: 0,
              }}
            >
              {spaceLabel}
            </p>
          </div>
        )}

        {/* Shared KR detail content */}
        <KRDetailBody kr={kr} />

        {/* All related entities grouped by type */}
        <RelatedSection items={relatedItems} grouped />
      </div>

      {/* Footer — same outline link button as MissionPanel */}
      <div
        style={{
          padding: '12px 16px',
          background: 'var(--bg-panel)',
          borderTop: '1px solid var(--border)',
        }}
      >
        <Button
          variant="outline"
          className="w-full gap-2"
          style={{
            fontFamily: SF_TEXT,
            fontSize: '14px',
            fontWeight: 400,
            lineHeight: 1.43,
            letterSpacing: '-0.224px',
            color: 'var(--apple-link)',
            borderColor: 'var(--apple-link)',
          }}
          onClick={() => navigate(missionDetailLink(projectId, kr.missionId))}
        >
          <ExternalLink size={12} />
          Open Mission Detail
        </Button>
      </div>
    </div>
  )
}
