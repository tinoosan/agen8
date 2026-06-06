import { useState, useMemo } from 'react'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useMissions, useProjectKRs } from '../../hooks/useMissions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@/components/ui/collapsible'
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from '@/components/ui/tooltip'
import { ScrollText, ChevronRight, AlertCircle, Link2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { DecisionView } from '../../lib/types'
import { safeReferenceLabel, sanitizeDecisionTitle } from '../../lib/displaySanitizers'
import DecisionDetails from '../decision/DecisionDetails'
import { sourceToClusterColor } from '../../lib/clusterColors'
import { decisionsLink } from '../../lib/routing'
import { decisionActorDisplay } from '../../lib/decisionDisplay'
import { formatRelative } from '@/lib/format'
import { Link } from 'wouter'

/* -- Confidence dots ------------------------------------------------------ */

function ConfidenceDots({ confidence }: { confidence: number }) {
  // 0..1 mapped to 1..5 filled dots
  const filled = Math.max(1, Math.min(5, Math.round(confidence * 5)))

  const color =
    confidence >= 0.8 ? 'bg-[var(--green)]' :
    confidence >= 0.5 ? 'bg-[var(--amber)]' :
    'bg-[var(--red)]'

  return (
    <TooltipProvider delayDuration={200}>
      <Tooltip>
        <TooltipTrigger asChild>
          <div className="flex items-center gap-0.5 shrink-0" aria-label={`Confidence: ${Math.round(confidence * 100)}%`}>
            {Array.from({ length: 5 }, (_, i) => (
              <span
                key={i}
                className={cn(
                  'w-1 h-1 rounded-full',
                  i < filled ? color : 'bg-[var(--text-3)]/30',
                )}
              />
            ))}
          </div>
        </TooltipTrigger>
        <TooltipContent side="top" className="text-[0.6875rem]">
          {Math.round(confidence * 100)}% confidence
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  )
}

/* -- Ref links (KR / Task / OA) ------------------------------------------ */

type DecisionRefCatalogs = {
  missionTitles: Map<string, string>
  keyResultTitles: Map<string, string>
  taskTitles: Map<string, string>
}

function firstSafeValue(...values: Array<string | null | undefined>): string | null {
  for (const value of values) {
    const safe = safeReferenceLabel(value)
    if (safe) return safe
  }
  return null
}

function resolveDecisionRef(
  decision: DecisionView,
  key: 'mission' | 'keyResult' | 'task',
  ref: string | null | undefined,
  catalogs: DecisionRefCatalogs,
): string | null {
  const trimmedRef = ref?.trim()
  if (!trimmedRef) return null

  const metadata = decision.metadata ?? {}
  const metadataLabel =
    key === 'mission' ? firstSafeValue(metadata.missionTitle, metadata['mission.title']) :
    key === 'keyResult' ? firstSafeValue(metadata.keyResultTitle, metadata.krTitle, metadata['keyResult.title']) :
    firstSafeValue(metadata.taskTitle, metadata['task.title'])

  if (metadataLabel) return metadataLabel

  const mapLabel =
    key === 'mission' ? catalogs.missionTitles.get(trimmedRef) :
    key === 'keyResult' ? catalogs.keyResultTitles.get(trimmedRef) :
    catalogs.taskTitles.get(trimmedRef)

  return firstSafeValue(mapLabel, trimmedRef)
}

function RefLinks({ decision, catalogs }: { decision: DecisionView; catalogs: DecisionRefCatalogs }) {
  const refs: { label: string; value: string }[] = []
  const mission = resolveDecisionRef(decision, 'mission', decision.missionRef, catalogs)
  const keyResult = resolveDecisionRef(decision, 'keyResult', decision.keyResultRef, catalogs)
  const task = resolveDecisionRef(decision, 'task', decision.taskRef, catalogs)
  if (mission) refs.push({ label: 'Mission', value: mission })
  if (keyResult) refs.push({ label: 'KR', value: keyResult })
  if (task) refs.push({ label: 'Task', value: task })
  if (refs.length === 0) return null

  return (
    <div className="flex items-center gap-1.5 flex-wrap">
      {refs.map(r => (
        <span key={`${r.label}:${r.value}`} className="decision-ref-link">
          <Link2 size={8} />
          {r.label}: {r.value}
        </span>
      ))}
    </div>
  )
}

function PrimaryRefLink({ decision, catalogs }: { decision: DecisionView; catalogs: DecisionRefCatalogs }) {
  const refs: { label: string; value: string }[] = []
  const mission = resolveDecisionRef(decision, 'mission', decision.missionRef, catalogs)
  const keyResult = resolveDecisionRef(decision, 'keyResult', decision.keyResultRef, catalogs)
  const task = resolveDecisionRef(decision, 'task', decision.taskRef, catalogs)
  if (mission) refs.push({ label: 'Mission', value: mission })
  if (keyResult) refs.push({ label: 'KR', value: keyResult })
  if (task) refs.push({ label: 'Task', value: task })
  const primary = refs[0]
  if (!primary) return null

  return (
    <span className="decision-ref-link decision-ref-link-primary">
      <Link2 size={8} className="shrink-0" />
      <span className="truncate min-w-0">{primary.label}: {primary.value}</span>
    </span>
  )
}

/* -- Temporal grouping ---------------------------------------------------- */

const MINUTE = 60_000
const HOUR = 3_600_000
const DAY = 86_400_000

type TimeBucket = 'Just now' | 'Last hour' | 'Today' | 'Yesterday' | 'Older'

const BUCKET_ORDER: TimeBucket[] = ['Just now', 'Last hour', 'Today', 'Yesterday', 'Older']

function getTimeBucket(iso: string): TimeBucket {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 5 * MINUTE) return 'Just now'
  if (diff < HOUR) return 'Last hour'
  if (diff < DAY) return 'Today'
  if (diff < 2 * DAY) return 'Yesterday'
  return 'Older'
}

function getRecencyClass(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  if (diff < 5 * MINUTE) return 'decision-recency-recent'
  if (diff < HOUR) return 'decision-recency-hour'
  if (diff < DAY) return 'decision-recency-day'
  return 'decision-recency-old'
}

function groupByTimeBucket(decisions: DecisionView[]): { bucket: TimeBucket; items: DecisionView[] }[] {
  const groups = new Map<TimeBucket, DecisionView[]>()
  for (const d of decisions) {
    const bucket = getTimeBucket(d.createdAt)
    if (!groups.has(bucket)) groups.set(bucket, [])
    groups.get(bucket)!.push(d)
  }
  return BUCKET_ORDER.filter(b => groups.has(b)).map(bucket => ({
    bucket,
    items: groups.get(bucket)!,
  }))
}

/* -- Single decision row -------------------------------------------------- */

function DecisionRow({ decision, catalogs }: { decision: DecisionView; catalogs: DecisionRefCatalogs }) {
  const [open, setOpen] = useState(false)
  const actor = decisionActorDisplay(decision)
  const baseClusterColor = actor.clusterKey
    ? sourceToClusterColor(actor.clusterKey)
    : 'var(--text-3)'
  const clusterColor = `color-mix(in srgb, ${baseClusterColor} 58%, var(--text-2) 42%)`
  const recencyClass = getRecencyClass(decision.createdAt)
  const identity = actor.label

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className={cn('decision-card', recencyClass)}>
        <CollapsibleTrigger asChild>
          <button className="flex items-center gap-3 w-full text-left bg-transparent border-none cursor-pointer p-0 font-[inherit] focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:outline-none rounded-[var(--r-sm)]">
            {/* Content column */}
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-[0.6875rem] font-semibold tracking-[-0.01em] truncate min-w-0" style={{ color: clusterColor }}>
                  {identity}
                </span>
                <span className="text-[0.625rem] text-[var(--text-3)] tabular-nums shrink-0">
                  {formatRelative(decision.createdAt, { seconds: true })}
                </span>
              </div>
              <div className="flex items-center gap-2 mt-0.5">
                <span className="text-[0.75rem] font-medium text-[var(--text-1)] truncate flex-1 min-w-0 leading-snug">
                  {sanitizeDecisionTitle(decision.title)}
                </span>
                <ConfidenceDots confidence={decision.confidence} />
              </div>
              <div className="mt-0.5">
                <PrimaryRefLink decision={decision} catalogs={catalogs} />
              </div>
            </div>

            {/* Expand chevron */}
            <ChevronRight
              size={12}
              className={cn(
                'text-[var(--text-3)] shrink-0 transition-transform duration-150 opacity-40',
                open && 'rotate-90 opacity-70',
              )}
            />
          </button>
        </CollapsibleTrigger>

        {/* Expanded rationale */}
        <CollapsibleContent>
            <div className="mt-2 ml-10">
              <div className="bg-[var(--bg-surface)] rounded-[var(--r-md)] p-2.5 text-[0.6875rem] text-[var(--text-2)] leading-[1.55]">
                <div className="mb-2">
                  <RefLinks decision={decision} catalogs={catalogs} />
                </div>
                <DecisionDetails decision={decision} />
              </div>
            </div>
        </CollapsibleContent>
      </div>
    </Collapsible>
  )
}

/* -- Loading skeleton ----------------------------------------------------- */

function DecisionFeedSkeleton() {
  return (
    <div className="dashboard-section">
      <div className="flex items-center gap-2 mb-3">
        <Skeleton className="h-4 w-4 rounded-full" />
        <Skeleton className="h-4 w-36" />
      </div>
      <div className="flex items-center gap-3 mb-4">
        {[1, 2, 3].map(i => (
          <div key={i} className="flex items-center gap-1.5">
            <Skeleton className="h-2 w-2 rounded-full" />
            <Skeleton className="h-3 w-12" />
          </div>
        ))}
      </div>
      <div className="flex flex-col gap-1">
        {[1, 2, 3].map(i => (
          <div key={i} className="flex items-center gap-3 py-2 px-2.5">
            <div className="flex-1">
              <Skeleton className="h-3 w-20 mb-1.5" />
              <Skeleton className="h-3.5 w-48" />
            </div>
          </div>
        ))}
      </div>
    </div>
  )
}

/* -- Main exported component ---------------------------------------------- */

export default function DecisionFeed({ projectId, hideHeader, defaultExpanded = false }: {
  projectId: string | null
  hideHeader?: boolean
  defaultExpanded?: boolean
}) {
  const { data: decisions, isLoading, isError, error } = useRecentDecisions(projectId)
  const missionsQuery = useMissions(projectId)
  const projectKRsQuery = useProjectKRs(projectId)
  const projectTasksQuery = useProjectTasks(projectId)

  // All hooks must be called before any early return (Rules of Hooks)
  const safeDecisions = useMemo(() => decisions ?? [], [decisions])
  const refCatalogs = useMemo<DecisionRefCatalogs>(() => {
    const missionTitles = new Map<string, string>()
    for (const mission of missionsQuery.data ?? []) {
      const label = safeReferenceLabel(mission.title)
      if (label) missionTitles.set(mission.id, label)
    }

    const keyResultTitles = new Map<string, string>()
    for (const [id, keyResult] of projectKRsQuery.data ?? new Map()) {
      const label = safeReferenceLabel(keyResult.title)
      if (label) keyResultTitles.set(id, label)
    }

    const taskTitles = new Map<string, string>()
    for (const task of projectTasksQuery.data ?? []) {
      const label = firstSafeValue(task.title, task.description)
      if (label) taskTitles.set(task.id, label)
    }

    return {
      missionTitles,
      keyResultTitles,
      taskTitles,
    }
  }, [
    missionsQuery.data,
    projectKRsQuery.data,
    projectTasksQuery.data,
  ])

  const PREVIEW_COUNT = 4
  const [expanded, setExpanded] = useState(defaultExpanded)
  const hasMore = safeDecisions.length > PREVIEW_COUNT

  // Early returns — all hooks already called above
  if (!projectId) return null

  if (isLoading) {
    return <DecisionFeedSkeleton />
  }

  if (isError) {
    return (
      <div className="dashboard-section flex items-center gap-2 px-1 py-3 text-xs text-[var(--red)]">
        <AlertCircle size={14} />
        <span>Failed to load decisions: {error instanceof Error ? error.message : 'Unknown error'}</span>
      </div>
    )
  }

  if (safeDecisions.length === 0) {
    if (!hideHeader) return null
    return (
      <div className="dashboard-section">
        <div className="flex items-center gap-2.5 px-1 py-3 text-[0.8125rem] text-[var(--text-3)]">
          <ScrollText size={16} className="opacity-40" />
          <span>No decisions recorded yet</span>
        </div>
      </div>
    )
  }

  return (
    <section className="dashboard-section">
      {/* Header */}
      {!hideHeader && (
        <div className="dashboard-section-heading mb-2">
          <div className="dashboard-section-heading-main">
            <div className="flex items-center gap-2">
              <ScrollText size={14} className="text-[var(--accent)]" />
              <span className="dashboard-section-title">
                Recent Decisions
              </span>
              <span className="decision-heading-count">
                {safeDecisions.length} latest
              </span>
            </div>
            <p className="dashboard-section-caption">What changed, and who shaped it.</p>
          </div>
        </div>
      )}
      {hideHeader && (
        <div className="mb-3 flex items-center gap-2">
          <div className="min-w-0 flex-1">
            <span className="decision-heading-count">
              {safeDecisions.length} latest
            </span>
          </div>
          <Button asChild variant="outline" size="xs" className="shrink-0">
            <Link to={decisionsLink(projectId)}>Open full log</Link>
          </Button>
        </div>
      )}

      {/* Decision feed — preview or full */}
      {expanded ? (
        <div className="flex flex-col max-h-[520px] overflow-y-auto overflow-x-hidden pr-1">
          {groupByTimeBucket(safeDecisions).map(({ bucket, items }, gi) => (
            <div key={bucket} className={cn(gi > 0 && 'mt-4')}>
              <div className="decision-time-divider">{bucket}</div>
              <div className="flex flex-col gap-1">
                {items.map(decision => (
                  <DecisionRow key={decision.id} decision={decision} catalogs={refCatalogs} />
                ))}
              </div>
            </div>
          ))}
        </div>
      ) : (
        <div className="flex flex-col gap-1">
          {safeDecisions.slice(0, PREVIEW_COUNT).map(decision => (
            <DecisionRow key={decision.id} decision={decision} catalogs={refCatalogs} />
          ))}
        </div>
      )}

      {/* Expand/collapse toggle */}
      {hasMore && (
        <button
          onClick={() => setExpanded(!expanded)}
          className="mt-3 text-[0.6875rem] font-medium text-[var(--accent)] hover:underline bg-transparent border-none cursor-pointer p-0 font-[inherit]"
        >
          {expanded ? 'Show less' : `Open full log (${safeDecisions.length})`}
        </button>
      )}
    </section>
  )
}
