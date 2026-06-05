import { useState } from 'react'
import { toast } from 'sonner'
import { X, ExternalLink, Calendar } from 'lucide-react'
import { useLocation } from 'wouter'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { Button } from '@/components/ui/button'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { useKeyResults, useUpdateMission } from '../../hooks/useMissions'
import { missionDetailLink } from '../../lib/routing'
import MissionLifecycleActions from '../mission/MissionLifecycleActions'
import { RelatedSection } from './RelatedSection'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import type { MissionNodeData } from './MissionNode'
import type { MissionStatus } from '../../lib/types'
import type { NodePanelProps } from './types'

const SF_TEXT = 'SF Pro Text, SF Pro Icons, Helvetica Neue, Helvetica, Arial, sans-serif'
const SUMMARY_MIN = 80
const SUMMARY_MAX = 480
const SUMMARY_DEFAULT = 220

const STATUS_DOT_COLOR: Record<MissionStatus, string> = {
  draft: 'var(--text-3)',
  active: 'var(--green)',
  paused: 'var(--amber)',
  completed: 'var(--accent)',
  archived: 'var(--text-3)',
}

export function MissionPanel({ data, projectId, onClose }: NodePanelProps) {
  const d = data as MissionNodeData
  const { mission, avgProgress } = d
  const clusterColor = d.clusterColor ?? 'var(--accent)'
  const krQuery = useKeyResults(mission.id)
  const krs = krQuery.data ?? []
  const [, navigate] = useLocation()
  const [summaryHeight, setSummaryHeight] = useState(() => {
    const stored = localStorage.getItem('mission-panel-summary-height')
    return stored ? Math.max(SUMMARY_MIN, Math.min(SUMMARY_MAX, parseInt(stored, 10))) : SUMMARY_DEFAULT
  })
  const updateMission = useUpdateMission()

  // Fetch all related entities for the grouped Related section
  const krIds = new Set(krs.map(kr => kr.id))
  const tasksQuery = useProjectTasks(projectId)
  const tasks = (tasksQuery.data ?? []).filter(t => t.keyResultRef && krIds.has(t.keyResultRef))
  const decisionsQuery = useRecentDecisions(projectId)
  const decisions = (decisionsQuery.data ?? []).filter(d => d.missionRef === mission.id || (d.keyResultRef && krIds.has(d.keyResultRef)))

  const relatedItems = [
    ...krs.map(kr => ({
      nodeId: kr.id,
      type: 'Key Result' as const,
      title: kr.title,
      badge: `${Math.round(kr.progressPercent)}%`,
    })),
    ...tasks.map(t => ({
      nodeId: `task:${t.id}`,
      type: 'Task' as const,
      title: t.title ?? t.description ?? t.id.slice(0, 12),
    })),
    ...decisions.map(dec => ({
      nodeId: `decision:${dec.id}`,
      type: 'Decision' as const,
      title: dec.title,
      ...(dec.confidence > 0 ? {
        badge: `${Math.round(dec.confidence * 100)}%`,
        badgeColor: dec.confidence >= 0.8 ? 'var(--green)' : dec.confidence >= 0.6 ? 'var(--amber)' : 'var(--red)',
      } : {}),
    })),
  ]

  async function handleStatusChange(status: MissionStatus) {
    try {
      await updateMission.mutateAsync({ missionId: mission.id, status })
      toast.success(`Mission ${status === 'active' ? 'activated' : status}`)
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to update mission status')
    }
  }

  const handleResizeStart = (e: React.MouseEvent) => {
    e.preventDefault()
    const startY = e.clientY
    const startH = summaryHeight

    const onMove = (ev: MouseEvent) => {
      const next = Math.max(SUMMARY_MIN, Math.min(SUMMARY_MAX, startH + ev.clientY - startY))
      setSummaryHeight(next)
      localStorage.setItem('mission-panel-summary-height', String(next))
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }

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
            style={{ fontSize: '10px', fontWeight: 500, letterSpacing: '0.08em', lineHeight: 1.33, color: 'var(--text-3)' }}
          >
            Mission
          </p>
          <h2
            className="text-foreground line-clamp-2"
            style={{ fontFamily: SF_TEXT, fontSize: '17px', fontWeight: 600, lineHeight: 1.24, letterSpacing: '-0.374px' }}
          >
            {mission.title}
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

      {/* Summary zone — caps at summaryHeight with scroll, shrinks to fit when content is short */}
      <div
        className="shrink-0 overflow-y-auto flex flex-col"
        style={{ maxHeight: summaryHeight, background: 'var(--bg-panel)', padding: '10px 16px 12px', gap: '12px' }}
      >
        {/* Lifecycle actions — above the status pill, only when transitions exist */}
        {mission.status !== 'archived' && (
          <MissionLifecycleActions
            mission={mission}
            onStatusChange={handleStatusChange}
            isPending={updateMission.isPending}
          />
        )}

        {/* Status + dates */}
        <div className="flex items-center gap-2 flex-wrap">
          <Badge
            variant="outline"
            className="gap-1.5"
            style={{ border: 'none', padding: 0, background: 'transparent', fontSize: '12px', fontWeight: 600, letterSpacing: '-0.12px', lineHeight: 1.33 }}
          >
            <span
              style={{ width: 6, height: 6, borderRadius: '50%', background: STATUS_DOT_COLOR[mission.status], flexShrink: 0, display: 'inline-block' }}
            />
            {mission.status}
          </Badge>
          {(mission.startDate || mission.endDate) && (
            <span
              className="flex items-center gap-1"
              style={{ fontSize: '10px', letterSpacing: '-0.08px', lineHeight: 1.47, color: 'var(--text-3)' }}
            >
              <Calendar size={10} />
              {mission.startDate?.slice(0, 10) ?? '—'} → {mission.endDate?.slice(0, 10) ?? '—'}
            </span>
          )}
        </div>

        {/* Description — full markdown, no cap; summary height controls visible area */}
        {mission.description && (
          <div
            className="md-prose"
            style={{ fontFamily: SF_TEXT, fontSize: '14px', letterSpacing: '-0.224px', color: 'var(--text-2)' }}
          >
            <ReactMarkdown remarkPlugins={[remarkGfm]}>
              {mission.description}
            </ReactMarkdown>
          </div>
        )}

      </div>

      {/* Progress — pinned, always visible regardless of summary scroll */}
      <div
        className="shrink-0 flex flex-col"
        style={{ background: 'var(--bg-panel)', padding: '10px 16px', gap: '5px' }}
      >
        <div className="flex justify-between items-baseline">
          <span style={{ fontSize: '11px', fontWeight: 400, letterSpacing: '-0.12px', lineHeight: 1.33, color: 'var(--text-3)' }}>
            Avg KR progress
          </span>
          <span style={{ fontSize: '11px', fontWeight: 600, letterSpacing: '-0.12px', lineHeight: 1.33, color: 'var(--text-2)' }}>
            {avgProgress}%
          </span>
        </div>
        <div style={{ height: '3px', borderRadius: '980px', background: 'var(--bg-elevated)', overflow: 'hidden' }}>
          <div
            style={{
              height: '100%',
              borderRadius: '980px',
              width: `${Math.min(100, Math.max(0, avgProgress))}%`,
              background: clusterColor,
              transition: 'width 0.3s ease',
            }}
          />
        </div>
      </div>

      {/* Drag handle */}
      <div
        className="shrink-0 flex items-center justify-center select-none cursor-ns-resize"
        style={{ height: 10, background: 'var(--bg-panel)', borderTop: '1px solid var(--border)', borderBottom: '1px solid var(--border)' }}
        onMouseDown={handleResizeStart}
      >
        <div style={{ width: 20, height: 2, borderRadius: 1, background: 'var(--border-strong)' }} />
      </div>

      {/* Related — all entities in this mission's cluster, grouped by type */}
      <div
        className="flex-1 overflow-y-auto"
        style={{ background: 'var(--bg-panel)', padding: '8px 16px', minHeight: 0 }}
      >
        {krQuery.isLoading && (
          <div className="flex flex-col" style={{ gap: '8px', paddingTop: '4px' }}>
            {Array.from({ length: 3 }).map((_, i) => (
              <Skeleton key={i} className="h-12 rounded-md" />
            ))}
          </div>
        )}
        <RelatedSection items={relatedItems} grouped />
      </div>

      {/* Footer */}
      <div style={{ padding: '12px 16px', background: 'var(--bg-panel)', borderTop: '1px solid var(--border)' }}>
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
          onClick={() => navigate(missionDetailLink(projectId, mission.id))}
        >
          <ExternalLink size={12} />
          Open Mission Detail
        </Button>
      </div>
    </div>
  )
}
