import { useState } from 'react'
import { useQuery } from '@tanstack/react-query'
import { useRoute, Link } from 'wouter'
import { rpcCall } from '../lib/rpc'
import { qk } from '../lib/queryKeys'
import { useKeyResults } from '../hooks/useMissions'
import { useProjectTasks } from '../hooks/useProjectTasks'
import { useRecentDecisions } from '../hooks/useDecisions'
import { entityDisplayTitle } from '../lib/displaySanitizers'
import { missionsPanelLink, taskDetailLink, decisionDetailLink } from '../lib/routing'
import { DetailNotFound, DetailError } from '../components/detail/DetailStates'
import { DetailSkeleton } from '../components/detail/DetailSkeleton'
import { DetailHeader } from '../components/detail/DetailHeader'
import { RelatedList } from '../components/detail/RelatedList'
import { KRRow } from '../components/mission/KRRow'
import { Badge } from '@/components/ui/badge'
import { Skeleton } from '@/components/ui/skeleton'
import { formatDate } from '@/lib/format'
import { confidenceColor } from '@/lib/decisionDisplay'
import { Calendar, ChevronsUpDown } from 'lucide-react'
import type { MissionView } from '../lib/types'

/* ── Status helper ──────────────────────────────────────── */

function missionStatusBadge(status: string): { variant: 'success' | 'warning' | 'info' | 'secondary' | 'accent'; label: string } {
  switch (status) {
    case 'active':    return { variant: 'success',   label: 'Active' }
    case 'paused':    return { variant: 'warning',   label: 'Paused' }
    case 'completed': return { variant: 'info',      label: 'Completed' }
    case 'archived':  return { variant: 'secondary', label: 'Archived' }
    default:          return { variant: 'accent',    label: 'Draft' }
  }
}

/* ── Loading skeleton ────────────────────────────────────── */

function MissionDetailSkeleton() {
  return (
    <DetailSkeleton>
      <div className="flex flex-col gap-0.5">
        {[1, 2, 3].map(i => (
          <div key={i} className="flex items-center gap-3 px-3 py-2">
            <Skeleton className="h-3 w-3 rounded-sm shrink-0" />
            <Skeleton className="h-3 flex-1 max-w-[280px]" />
            <Skeleton className="h-3 w-8 ml-auto" />
            <Skeleton className="h-4 w-14 rounded-full" />
          </div>
        ))}
      </div>
    </DetailSkeleton>
  )
}

/* ── Main component ──────────────────────────────────────── */

export default function MissionDetail() {
  const [, params] = useRoute('/project/:projectId/missions/:missionId')
  const projectId  = params?.projectId  ? decodeURIComponent(params.projectId)  : null
  const missionId  = params?.missionId  ? decodeURIComponent(params.missionId)  : null
  const [expandedKrIds, setExpandedKrIds] = useState<Record<string, boolean>>({})

  const { data: mission, isLoading: missionLoading, isError: missionError, error: missionErr } =
    useQuery<MissionView>({
      queryKey: qk.missionGet(missionId),
      queryFn: async () => {
        const res = await rpcCall<{ mission: MissionView }>('mission.get', { missionId: missionId ?? '' })
        return res.mission
      },
      enabled: !!missionId,
      refetchInterval: 10_000,
    })

  const { data: keyResults, isLoading: krsLoading, isError: krsError, error: krsErr } =
    useKeyResults(missionId)

  // Related cross-references — the same set the strategy map's mission panel
  // surfaces. Called unconditionally (rules of hooks); both no-op until
  // projectId resolves.
  const { data: projectTasks } = useProjectTasks(projectId)
  const { data: projectDecisions } = useRecentDecisions(projectId)

  if (!projectId || !missionId) {
    return <DetailNotFound entity="mission" />
  }

  if (missionLoading || krsLoading) return <MissionDetailSkeleton />

  if (missionError || krsError) {
    const msg = missionErr instanceof Error ? missionErr.message
      : krsErr instanceof Error ? (krsErr as Error).message : 'Unknown error'
    return <DetailError entity="mission" message={msg} />
  }

  if (!mission) {
    return <DetailNotFound entity="mission" />
  }

  // Related cross-references (tasks + decisions). Filtering mirrors the strategy
  // map mission panel; rendered as the same flat, per-row-labelled list the task
  // and decision detail pages use.
  const krIds = new Set((keyResults ?? []).map((kr) => kr.id))
  const relatedTasks = (projectTasks ?? []).filter((t) => t.keyResultRef && krIds.has(t.keyResultRef))
  const relatedDecisions = (projectDecisions ?? []).filter(
    (d) => d.missionRef === mission.id || (d.keyResultRef && krIds.has(d.keyResultRef)),
  )
  const related: Array<{ key: string; label: string; title: string; to: string; suffix?: string; suffixColor?: string }> = []
  for (const t of relatedTasks) {
    related.push({
      key: t.id,
      label: 'Task',
      title: entityDisplayTitle(t.id, t.title, t.description),
      to: taskDetailLink(projectId, t.id),
    })
  }
  for (const d of relatedDecisions) {
    related.push({
      key: d.id,
      label: 'Decision',
      title: d.title,
      to: decisionDetailLink(projectId, d.id),
      ...(d.confidence > 0
        ? {
            suffix: `${Math.round(d.confidence * 100)}%`,
            suffixColor: confidenceColor(d.confidence),
          }
        : {}),
    })
  }

  const statusBadge = missionStatusBadge(mission.status)
  const overallProgress = keyResults && keyResults.length > 0
    ? keyResults.reduce((sum, kr) => sum + kr.progressPercent, 0) / keyResults.length
    : 0
  return (
    <div className="flex flex-col h-full overflow-y-auto">
      {/* Sticky header — full-width outer for background coverage, max-w inner for centering */}
      <DetailHeader backTo={missionsPanelLink(projectId)} backLabel="Missions">

          {/* Title row */}
          <div className="flex items-center gap-2.5 mb-1">
            <h1
              className="m-0 text-[var(--text-1)] flex-1 min-w-0"
              style={{ fontSize: '1.75rem', fontWeight: 700, letterSpacing: '-0.56px', lineHeight: 1.14 }}
            >
              {mission.title}
            </h1>
            <Badge variant={statusBadge.variant} className="shrink-0">
              {statusBadge.label}
            </Badge>
            {keyResults && keyResults.length > 0 && overallProgress > 0 && (
              <span className="tabular-nums text-[var(--text-3)] shrink-0" style={{ fontSize: '0.8125rem', fontWeight: 500, letterSpacing: '-0.06px' }}>
                {Math.round(overallProgress)}%
              </span>
            )}
          </div>

          {/* Description */}
          {mission.description && (
            <p className="mt-2 mb-0 text-[var(--text-3)] ml-[18px]" style={{ fontSize: '0.875rem', letterSpacing: '-0.14px', lineHeight: 1.5 }}>
              {mission.description}
            </p>
          )}

          {/* Metadata */}
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1 mt-2.5 ml-[18px]">
            {(mission.startDate || mission.endDate) && (
              <span className="inline-flex items-center gap-1 text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}>
                <Calendar size={11} />
                {mission.startDate && mission.endDate
                  ? `${formatDate(mission.startDate, { style: 'numeric' })} – ${formatDate(mission.endDate, { style: 'numeric' })}`
                  : mission.endDate
                    ? `Due ${formatDate(mission.endDate, { style: 'numeric' })}`
                    : 'No deadline'}
              </span>
            )}
            {keyResults && keyResults.length > 0 && (
              <span className="text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}>
                {keyResults.length} KR{keyResults.length !== 1 ? 's' : ''}
              </span>
            )}
          </div>

      </DetailHeader>

      {/* Scrollable KR list */}
      <div className="px-6 py-5 max-w-4xl mx-auto w-full">
        <div className="flex items-center gap-1.5 mb-2">
          <span className="text-[var(--text-3)]" style={{ fontSize: '0.6875rem', fontWeight: 500, letterSpacing: '-0.04px' }}>
            Key Results
          </span>
          {keyResults && keyResults.length > 0 && (
            <span className="text-[0.6875rem] text-[var(--text-3)] tabular-nums">{keyResults.length}</span>
          )}
          {keyResults && keyResults.length > 0 && (
            <button
              className="ml-auto inline-flex items-center gap-1 text-[var(--text-3)] hover:text-[var(--text-2)] transition-colors bg-transparent border-none cursor-pointer p-0"
              style={{ fontSize: '0.6875rem', letterSpacing: '-0.06px' }}
              onClick={() => {
                const allExpanded = keyResults.every(kr => !!expandedKrIds[kr.id])
                const newState = !allExpanded
                setExpandedKrIds(prev => {
                  const next = { ...prev }
                  for (const kr of keyResults) next[kr.id] = newState
                  return next
                })
              }}
            >
              <ChevronsUpDown size={11} />
              {keyResults.every(kr => !!expandedKrIds[kr.id]) ? 'Collapse all' : 'Expand all'}
            </button>
          )}
        </div>

        {(!keyResults || keyResults.length === 0) ? (
          <div className="rounded-[8px] bg-[var(--bg-surface)] px-4 py-8 text-center">
            <p className="text-[var(--text-3)] m-0" style={{ fontSize: '0.8125rem', letterSpacing: '-0.08px' }}>
              No key results defined yet.{' '}
              <Link
                to={missionsPanelLink(projectId)}
                className="text-[var(--accent)]"
              >
                Add from the missions list.
              </Link>
            </p>
          </div>
        ) : (
          <div className="flex flex-col gap-0.5">
            {keyResults.map(kr => (
              <KRRow
                key={kr.id}
                kr={kr}
                missionId={missionId}
                expanded={!!expandedKrIds[kr.id]}
                onExpandedChange={(v) => setExpandedKrIds(prev => ({ ...prev, [kr.id]: v }))}
              />
            ))}
          </div>
        )}
      </div>

      {/* Related — tasks + decisions linked to this mission, rendered with the
          same flat, per-row-labelled list the task and decision detail pages use */}
      {related.length > 0 && (
        <div className="px-6 pb-8 max-w-4xl mx-auto w-full">
          <RelatedList items={related} storageKey="mission-detail-related" />
        </div>
      )}
    </div>
  )
}
