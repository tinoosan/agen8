import { useState, useMemo } from 'react'
import { useRecentDecisions, type DecisionListFilter } from '../../hooks/useDecisions'
import { useMissions, useProjectKRs } from '../../hooks/useMissions'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { usePendingOpActions } from '../../hooks/useOpActions'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Collapsible, CollapsibleTrigger, CollapsibleContent } from '@/components/ui/collapsible'
import { Tooltip, TooltipTrigger, TooltipContent, TooltipProvider } from '@/components/ui/tooltip'
import { ScrollText, ChevronRight, AlertCircle, Link2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { DecisionView, ProjectSpaceSummary } from '../../lib/types'
import { spaceDisplayName } from '../../lib/spaceDisplayName'
import { safeReferenceLabel, sanitizeDecisionTitle } from '../../lib/displaySanitizers'
import DecisionDetails from '../decision/DecisionDetails'
import { sourceToClusterColor } from '../../lib/clusterColors'
import { decisionsLink } from '../../lib/routing'
import { Link } from 'wouter'

/* -- Time-ago helper ------------------------------------------------------ */

function timeAgo(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return 'just now'
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  const days = Math.floor(hours / 24)
  return `${days}d ago`
}

function compactRef(ref: string): string {
  if (ref.length <= 24) return ref
  return `${ref.slice(0, 12)}…${ref.slice(-6)}`
}

function compactRole(role: string): string {
  if (role.length <= 14) return role
  return `${role.slice(0, 10)}…`
}

function spaceSummaryDetail(roles: Array<{ role: string; count: number }>): string | null {
  const lead = roles[0]
  if (!lead) return null
  if (roles.length === 1) return `${compactRole(lead.role)} shapes this`
  return `${compactRole(lead.role)} leads · +${roles.length - 1} more`
}

function displaySpaceLabel(spaceLabel: string): string {
  const ref = spaceLabel.trim()
  if (!ref || ref === 'unknown') return 'Unscoped'
  if (/^space-[0-9a-f-]{8,}$/i.test(ref)) return 'Archived space'
  return spaceDisplayName(undefined, ref)
}

function decisionSpaceLabel(decision: DecisionView, spaceLabelByOwnerId?: Map<string, string>): string {
  const spaceLabel = decision.spaceName?.trim()
  if (spaceLabel) return spaceLabel
  const mappedLabel = decision.spaceId ? spaceLabelByOwnerId?.get(decision.spaceId)?.trim() : ''
  if (mappedLabel) return mappedLabel
  return 'unknown'
}

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
        <TooltipContent side="top" className="text-[11px]">
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
  operatorActionTitles: Map<string, string>
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
  key: 'mission' | 'keyResult' | 'task' | 'operatorAction',
  ref: string | null | undefined,
  catalogs: DecisionRefCatalogs,
): string | null {
  const trimmedRef = ref?.trim()
  if (!trimmedRef) return null

  const metadata = decision.metadata ?? {}
  const metadataLabel =
    key === 'mission' ? firstSafeValue(metadata.missionTitle, metadata['mission.title']) :
    key === 'keyResult' ? firstSafeValue(metadata.keyResultTitle, metadata.krTitle, metadata['keyResult.title']) :
    key === 'task' ? firstSafeValue(metadata.taskTitle, metadata['task.title']) :
    firstSafeValue(metadata.operatorActionTitle, metadata.actionTitle, metadata['operatorAction.title'])

  if (metadataLabel) return metadataLabel

  const mapLabel =
    key === 'mission' ? catalogs.missionTitles.get(trimmedRef) :
    key === 'keyResult' ? catalogs.keyResultTitles.get(trimmedRef) :
    key === 'task' ? catalogs.taskTitles.get(trimmedRef) :
    catalogs.operatorActionTitles.get(trimmedRef)

  return firstSafeValue(mapLabel, trimmedRef)
}

function RefLinks({ decision, catalogs }: { decision: DecisionView; catalogs: DecisionRefCatalogs }) {
  const refs: { label: string; value: string }[] = []
  const mission = resolveDecisionRef(decision, 'mission', decision.missionRef, catalogs)
  const keyResult = resolveDecisionRef(decision, 'keyResult', decision.keyResultRef, catalogs)
  const task = resolveDecisionRef(decision, 'task', decision.taskRef, catalogs)
  const operatorAction = resolveDecisionRef(decision, 'operatorAction', decision.operatorActionRef, catalogs)
  if (mission) refs.push({ label: 'Mission', value: mission })
  if (keyResult) refs.push({ label: 'KR', value: keyResult })
  if (task) refs.push({ label: 'Task', value: task })
  if (operatorAction) refs.push({ label: 'Action', value: operatorAction })
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
  const operatorAction = resolveDecisionRef(decision, 'operatorAction', decision.operatorActionRef, catalogs)
  if (mission) refs.push({ label: 'Mission', value: mission })
  if (keyResult) refs.push({ label: 'KR', value: keyResult })
  if (task) refs.push({ label: 'Task', value: task })
  if (operatorAction) refs.push({ label: 'Action', value: operatorAction })
  const primary = refs[0]
  if (!primary) return null

  return (
    <span className="decision-ref-link decision-ref-link-primary">
      <Link2 size={8} />
      {primary.label}: {primary.value}
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
  // Cluster color and identity label both prefer the resolved member
  // name; fall back to the raw id only when the registry lookup
  // couldn't resolve it. The bare sourceIdentity uuid is no longer
  // surfaced as a label.
  const memberKey = decision.memberId?.trim() || decision.sourceIdentity?.trim() || ''
  const memberLabel = decision.memberName?.trim() || memberKey
  const baseClusterColor = memberKey
    ? sourceToClusterColor(memberKey)
    : 'var(--text-3)'
  const clusterColor = `color-mix(in srgb, ${baseClusterColor} 58%, var(--text-2) 42%)`
  const recencyClass = getRecencyClass(decision.createdAt)
  const identity = memberLabel || (decision.source === 'operator' ? 'you' : 'agent')
  const avatarStyle = {
    color: clusterColor,
    background: `color-mix(in srgb, ${baseClusterColor} 9%, var(--bg-panel) 91%)`,
    borderColor: `color-mix(in srgb, ${baseClusterColor} 16%, transparent)`,
  }

  return (
    <Collapsible open={open} onOpenChange={setOpen}>
      <div className={cn('decision-card', recencyClass)}>
        <CollapsibleTrigger asChild>
          <button className="flex items-center gap-3 w-full text-left bg-transparent border-none cursor-pointer p-0 font-[inherit] focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:outline-none rounded-[var(--r-sm)]">
            {/* Role avatar — cluster-coloured, acts as visual anchor */}
            <span
              className="decision-identity-badge w-6 h-6 rounded-[10px] flex items-center justify-center text-[9px] font-semibold uppercase shrink-0 tracking-[0.02em]"
              style={avatarStyle}
            >
              {identity.slice(0, 2)}
            </span>

            {/* Content column */}
            <div className="flex-1 min-w-0">
              <div className="flex items-center gap-2">
                <span className="text-[11px] font-semibold tracking-[-0.01em]" style={{ color: clusterColor }}>
                  {identity}
                </span>
                <span className="text-[10px] text-[var(--text-3)] tabular-nums">
                  {timeAgo(decision.createdAt)}
                </span>
              </div>
              <div className="flex items-center gap-2 mt-0.5">
                <span className="text-[12px] font-medium text-[var(--text-1)] truncate flex-1 min-w-0 leading-snug">
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
              <div className="bg-[var(--bg-surface)] rounded-[var(--r-md)] p-2.5 text-[11px] text-[var(--text-2)] leading-[1.55]">
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
            <Skeleton className="h-7 w-7 rounded-[var(--r-md)]" />
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

type SourceFilter = '' | 'agent' | 'operator'

const SOURCE_FILTERS: { value: SourceFilter; label: string }[] = [
  { value: '', label: 'All' },
  { value: 'agent', label: 'Agent' },
  { value: 'operator', label: 'Operator' },
]

export default function DecisionFeed({ projectId, hideHeader, spaceLabelByOwnerId, spaces = [], defaultExpanded = false }: {
  projectId: string | null
  hideHeader?: boolean
  spaceLabelByOwnerId?: Map<string, string>
  spaces?: ProjectSpaceSummary[]
  defaultExpanded?: boolean
}) {
  const [sourceFilter, setSourceFilter] = useState<SourceFilter>('')
  const filter: DecisionListFilter | undefined = sourceFilter ? { source: sourceFilter as DecisionListFilter['source'] } : undefined
  const { data: decisions, isLoading, isError, error } = useRecentDecisions(
    projectId,
    filter,
  )
  const missionsQuery = useMissions(projectId)
  const projectKRsQuery = useProjectKRs(projectId)
  const projectTasksQuery = useProjectTasks(spaces)
  const opActionsQuery = usePendingOpActions(projectId)

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

    const operatorActionTitles = new Map<string, string>()
    for (const action of opActionsQuery.data ?? []) {
      const label = safeReferenceLabel(action.title)
      if (label) operatorActionTitles.set(action.id, label)
    }

    return {
      missionTitles,
      keyResultTitles,
      taskTitles,
      operatorActionTitles,
    }
  }, [
    missionsQuery.data,
    opActionsQuery.data,
    projectKRsQuery.data,
    projectTasksQuery.data,
  ])

  const spaceStats = useMemo(() => {
    const spaces = new Map<string, Map<string, number>>()
    for (const d of safeDecisions) {
      const ref = decisionSpaceLabel(d, spaceLabelByOwnerId)
      // "shapes this" attribution prefers the resolved name; the
      // memberId is a stable key so it remains a useful tie-breaker
      // when multiple anonymous members share the same source label.
      const role = d.memberName?.trim() || d.memberId?.trim() || d.sourceIdentity?.trim() || d.source
      if (!spaces.has(ref)) spaces.set(ref, new Map())
      const roles = spaces.get(ref)!
      roles.set(role, (roles.get(role) ?? 0) + 1)
    }
    return Array.from(spaces.entries())
      .map(([spaceLabel, roles]) => ({
        spaceLabel,
        displayName: displaySpaceLabel(spaceLabel),
        total: Array.from(roles.values()).reduce((a, b) => a + b, 0),
        roles: Array.from(roles.entries())
          .map(([role, count]) => ({ role, count, color: `color-mix(in srgb, ${sourceToClusterColor(role)} 58%, var(--text-2) 42%)` }))
          .sort((a, b) => b.count - a.count),
      }))
      .sort((a, b) => b.total - a.total)
  }, [safeDecisions, spaceLabelByOwnerId])

  const PREVIEW_COUNT = 4
  const SUMMARY_SPACE_LIMIT = 3
  const [expanded, setExpanded] = useState(defaultExpanded)
  const hasMore = safeDecisions.length > PREVIEW_COUNT
  const hiddenSpaceCount = Math.max(0, spaceStats.length - SUMMARY_SPACE_LIMIT)

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
    if (!hideHeader && !sourceFilter) return null
    return (
      <div className="dashboard-section">
        {hideHeader && (
          <div className="flex justify-end mb-3">
            <FilterButtons value={sourceFilter} onChange={setSourceFilter} />
          </div>
        )}
        {!hideHeader && sourceFilter && (
          <div className="flex items-center gap-2 mb-3">
            <ScrollText size={14} className="text-[var(--accent)]" />
            <span className="text-sm font-semibold text-[var(--text-1)] tracking-[-0.02em]">
              Decision Log
            </span>
            <div className="flex-1" />
            <FilterButtons value={sourceFilter} onChange={setSourceFilter} />
          </div>
        )}
        <div className="flex items-center gap-2.5 px-1 py-3 text-[13px] text-[var(--text-3)]">
          <ScrollText size={16} className="opacity-40" />
          <span>No {sourceFilter ? `${sourceFilter} ` : ''}decisions recorded yet</span>
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
          <div className="dashboard-section-meta">
            <FilterButtons value={sourceFilter} onChange={setSourceFilter} />
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
          <FilterButtons value={sourceFilter} onChange={setSourceFilter} />
          <Button asChild variant="outline" size="xs" className="shrink-0">
            <Link to={decisionsLink(projectId)}>Open full log</Link>
          </Button>
        </div>
      )}

      <div className="decision-space-summary mb-4">
        {spaceStats.slice(0, SUMMARY_SPACE_LIMIT).map(({ spaceLabel, displayName, total, roles }) => (
          <div
            key={spaceLabel}
            className="decision-space-summary-row"
            title={roles.map(({ role, count }) => `${role} ${count}`).join(' · ')}
          >
            <div className="decision-space-summary-head">
              <span className="decision-space-summary-name">{compactRef(displayName)}</span>
              <span className="decision-space-summary-total">{total}</span>
            </div>
            {roles[0] && (
              <div className="decision-space-summary-roles">
                <span className="decision-space-summary-dot" style={{ backgroundColor: roles[0].color }} />
                <span className="decision-space-summary-role-label">
                  {spaceSummaryDetail(roles)}
                </span>
              </div>
            )}
          </div>
        ))}
        {hiddenSpaceCount > 0 && (
          <span className="decision-space-summary-more">
            +{hiddenSpaceCount} more {hiddenSpaceCount === 1 ? 'space' : 'spaces'}
          </span>
        )}
      </div>

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
          className="mt-3 text-[11px] font-medium text-[var(--accent)] hover:underline bg-transparent border-none cursor-pointer p-0 font-[inherit]"
        >
          {expanded ? 'Show less' : `Open full log (${safeDecisions.length})`}
        </button>
      )}
    </section>
  )
}

function FilterButtons({ value, onChange }: { value: SourceFilter; onChange: (v: SourceFilter) => void }) {
  return (
    <div className="decision-filter-group">
      {SOURCE_FILTERS.map(f => (
        <Button
          key={f.value}
          variant="ghost"
          size="xs"
          onClick={() => onChange(f.value)}
          className={cn(
            'decision-filter-button text-[10px] px-2 py-0.5 rounded-[var(--r-sm)] transition-colors',
            value === f.value
              ? 'bg-[var(--bg-hover)] text-[var(--text-1)]'
              : 'text-[var(--text-3)] hover:text-[var(--text-2)]',
          )}
        >
          {f.label}
        </Button>
      ))}
    </div>
  )
}
