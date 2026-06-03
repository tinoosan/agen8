import { useEffect, useState } from 'react'
import { useOpAction, usePendingOpActions, useCompletedOpActions } from '../../hooks/useOpActions'
import { usePendingEscalations, useResolvedEscalations } from '../../hooks/useEscalations'
import { ResolveDialog } from './EscalationQueue'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { ArrowLeft, ClipboardList, Clock, User, AlertCircle, ChevronRight, ChevronDown, Link2 } from 'lucide-react'
import { cn } from '@/lib/utils'
import {
  LifecycleActions,
  LifecycleTimeline,
  MetaItem,
  NotesSection,
  OutcomeForm,
} from '../actions/OpActionContent'
import { statusConfig, timeAgo } from '../actions/opActionUtils'
import { safeReferenceLabel } from '../../lib/displaySanitizers'
import type { OpActionView, EscalationView, OperatorUrgency, OpActionStatus } from '../../lib/types'

type TypeFilter = 'all' | 'oa' | 'escalation'

const TYPE_FILTERS: { value: TypeFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'oa', label: 'Operator' },
  { value: 'escalation', label: 'Escalations' },
]

const URGENCY_ORDER: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 }
const URGENCY_TIERS: OperatorUrgency[] = ['critical', 'high', 'medium', 'low']

type ActionItem =
  | { kind: 'oa'; item: OpActionView }
  | { kind: 'escalation'; item: EscalationView }

function itemUrgency(item: ActionItem): string {
  return item.item.urgency
}

function itemCreatedAt(item: ActionItem): string {
  return item.item.createdAt
}

function sortItems(items: ActionItem[]): ActionItem[] {
  return [...items].sort((a, b) => {
    const ua = URGENCY_ORDER[itemUrgency(a)] ?? 4
    const ub = URGENCY_ORDER[itemUrgency(b)] ?? 4
    if (ua !== ub) return ua - ub
    return new Date(itemCreatedAt(a)).getTime() - new Date(itemCreatedAt(b)).getTime()
  })
}

function urgencyTone(urgency: OperatorUrgency): string {
  switch (urgency) {
    case 'critical': return 'dashboard-meta-chip-critical'
    case 'high': return 'dashboard-meta-chip-warning'
    case 'medium': return 'dashboard-meta-chip-info'
    default: return 'dashboard-meta-chip-muted'
  }
}

function oaStatusTone(status: OpActionStatus): string {
  switch (status) {
    case 'pending': return 'dashboard-meta-chip-warning'
    case 'acknowledged': return 'dashboard-meta-chip-info'
    case 'in_progress': return 'dashboard-meta-chip-accent'
    case 'pending_verification': return 'dashboard-meta-chip-warning'
    case 'completed': return 'dashboard-meta-chip-success'
    case 'blocked': return 'dashboard-meta-chip-critical'
    default: return 'dashboard-meta-chip-muted'
  }
}

function urgencyDotColor(urgency: OperatorUrgency): string {
  switch (urgency) {
    case 'critical': return 'var(--red)'
    case 'high': return 'var(--amber)'
    case 'medium': return 'var(--accent)'
    default: return 'var(--text-3)'
  }
}

function OARow({ oa, onClick }: { oa: OpActionView; onClick: () => void }) {
  const safeCategory = safeReferenceLabel(oa.category)
  const safeRole = safeReferenceLabel(oa.sourceMemberLabel)

  return (
    <button
      onClick={onClick}
      className={cn(
        'dashboard-queue-row flex w-full items-center gap-3 bg-transparent px-3 py-2.5 text-left',
        'border-0 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)] last:border-b-0 cursor-pointer font-[inherit]',
      )}
    >
      <span className={cn('dashboard-meta-chip dashboard-meta-chip-compact shrink-0', oaStatusTone(oa.status))}>
        {statusConfig(oa.status).label}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 min-w-0">
          <span className="truncate text-[12px] font-medium text-[var(--text-1)]">{oa.title}</span>
          {safeCategory && <span className="dashboard-inline-label shrink-0">{safeCategory}</span>}
        </div>
        {safeRole && (
          <div className="mt-0.5 flex items-center gap-1 text-[10px] text-[var(--text-3)]">
            <User size={10} />
            <span className="truncate">{safeRole}</span>
          </div>
        )}
      </div>
      <div className="flex items-center gap-1 shrink-0 text-[var(--text-3)]">
        <Clock size={10} />
        <span className="text-[10px] tabular-nums">{timeAgo(oa.createdAt)}</span>
      </div>
      <ChevronRight size={12} className="dashboard-space-chevron shrink-0 text-[var(--text-3)]" />
    </button>
  )
}

function EscalationRow({ esc, onClick }: { esc: EscalationView; onClick: () => void }) {
  const safeCategory = safeReferenceLabel(esc.category)
  const safeRole = safeReferenceLabel(esc.sourceMemberLabel)

  return (
    <button
      onClick={onClick}
      className={cn(
        'dashboard-queue-row flex w-full items-center gap-3 bg-transparent px-3 py-2.5 text-left',
        'border-0 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)] last:border-b-0 cursor-pointer font-[inherit]',
      )}
    >
      <span className={cn('dashboard-meta-chip dashboard-meta-chip-compact shrink-0', urgencyTone(esc.urgency))}>
        {esc.urgency === 'critical' ? 'Critical' : esc.urgency === 'high' ? 'High' : esc.urgency === 'medium' ? 'Medium' : 'Low'}
      </span>
      <div className="min-w-0 flex-1">
        <div className="flex items-center gap-2 min-w-0">
          <span className="truncate text-[12px] font-medium text-[var(--text-1)]">{esc.title}</span>
          {safeCategory && <span className="dashboard-inline-label shrink-0">{safeCategory}</span>}
        </div>
        {safeRole && (
          <div className="mt-0.5 flex items-center gap-1 text-[10px] text-[var(--text-3)]">
            <User size={10} />
            <span className="truncate">{safeRole}</span>
          </div>
        )}
      </div>
      <div className="flex items-center gap-1 shrink-0 text-[var(--text-3)]">
        <Clock size={10} />
        <span className="text-[10px] tabular-nums">{timeAgo(esc.createdAt)}</span>
      </div>
      <ChevronRight size={12} className="dashboard-space-chevron shrink-0 text-[var(--text-3)]" />
    </button>
  )
}

function ActionsSkeleton() {
  return (
    <div className="flex flex-col gap-6 px-1">
      {(['critical', 'high', 'medium'] as const).map((tier) => (
        <div key={tier}>
          <div className="mb-3 flex items-center gap-2">
            <Skeleton className="h-3 w-3 rounded-full" />
            <Skeleton className="h-4 w-20" />
          </div>
          <div className="dashboard-list-surface dashboard-list-surface-flat overflow-hidden">
            {[1, 2, 3].map((i) => (
              <div key={i} className="flex items-center gap-3 px-3 py-2.5 border-b border-[var(--border)] last:border-b-0">
                <Skeleton className="h-4 w-14 rounded-full" />
                <Skeleton className="h-4 flex-1" />
                <Skeleton className="h-4 w-10" />
              </div>
            ))}
          </div>
        </div>
      ))}
    </div>
  )
}

function ActionDetailInline({ actionId, onBack }: { actionId: string; onBack: () => void }) {
  const { data: oa, isLoading, isError } = useOpAction(actionId)
  const [showOutcomeForm, setShowOutcomeForm] = useState(false)

  if (isLoading) {
    return (
      <div className="p-5 space-y-4">
        <Skeleton className="h-4 w-24" />
        <Skeleton className="h-6 w-3/4" />
        <Skeleton className="h-20 w-full rounded-[var(--r-md)]" />
      </div>
    )
  }

  if (isError || !oa) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-5 text-center">
        <AlertCircle size={28} className="text-[var(--text-3)] opacity-70" />
        <p className="m-0 text-sm text-[var(--text-2)]">This action is no longer available.</p>
        <Button variant="outline" size="sm" onClick={onBack}>Back to actions</Button>
      </div>
    )
  }

  const sc = statusConfig(oa.status)
  const safeTask = safeReferenceLabel(oa.taskRef)
  const safeCategory = safeReferenceLabel(oa.category)
  const safeRole = safeReferenceLabel(oa.sourceMemberLabel)

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className="border-b border-[color-mix(in_srgb,var(--border)_48%,transparent)] px-5 py-4">
        <button
          onClick={onBack}
          className="mb-3 inline-flex items-center gap-1.5 border-0 bg-transparent p-0 text-[11px] text-[var(--text-3)] cursor-pointer hover:text-[var(--text-2)]"
        >
          <ArrowLeft size={12} />
          Back to actions
        </button>
        <div className="flex items-center gap-2 flex-wrap">
          <span
            className="rounded-full px-2 py-0.5 text-[10px] font-semibold"
            style={{ color: sc.color, background: sc.bg }}
          >
            {sc.label}
          </span>
          {oa.urgency === 'critical' && <span className="dashboard-inline-label dashboard-inline-label-critical">Critical</span>}
          {oa.urgency === 'high' && <span className="dashboard-inline-label dashboard-inline-label-warning">High urgency</span>}
          {oa.blocking && <span className="dashboard-inline-label">Blocking</span>}
        </div>
        <h2 className="m-0 mt-2 text-[18px] font-semibold tracking-[-0.03em] text-[var(--text-1)] leading-tight">{oa.title}</h2>
      </div>

      <div className="flex-1 min-h-0 overflow-y-auto px-5 py-4 space-y-5">
        <LifecycleTimeline status={oa.status} requiresVerification={oa.requiresVerification} />

        {showOutcomeForm && <OutcomeForm actionId={oa.id} onCancel={() => setShowOutcomeForm(false)} />}

        {oa.outcomeSummary && (
          <div className={cn(
            'rounded-[var(--r-md)] border px-3 py-3',
            oa.outcomeStatus === 'failed'
              ? 'bg-[var(--red-dim)] border-[var(--red)]/20'
              : oa.outcomeStatus === 'partial'
                ? 'bg-[var(--amber-dim)] border-[var(--amber)]/20'
                : 'bg-[var(--green-dim)] border-[var(--green)]/20',
          )}>
            <div className="text-[10px] font-semibold uppercase tracking-[0.05em] text-[var(--text-3)] mb-1">
              {oa.outcomeStatus ?? 'completed'}
            </div>
            <div className="text-xs text-[var(--text-2)] leading-relaxed">{oa.outcomeSummary}</div>
          </div>
        )}

        <div>
          <div className="mb-1.5 text-[9px] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]">Description</div>
          <div className="text-xs text-[var(--text-2)] leading-relaxed whitespace-pre-wrap">
            {oa.description ?? <span className="italic text-[var(--text-3)]">No description provided.</span>}
          </div>
        </div>

        <div className="grid grid-cols-2 gap-x-4 gap-y-2">
          {safeCategory && <MetaItem label="Category" value={safeCategory} />}
          {safeRole && <MetaItem label="Requested by" value={safeRole} icon={<User size={10} />} />}
          {safeTask && <MetaItem label="Task" value={safeTask} icon={<Link2 size={10} />} />}
          <MetaItem label="Created" value={timeAgo(oa.createdAt)} />
          {oa.startedAt && <MetaItem label="Started" value={timeAgo(oa.startedAt)} />}
          {oa.completedAt && <MetaItem label="Completed" value={timeAgo(oa.completedAt)} />}
          {oa.deadline && <MetaItem label="Deadline" value={new Date(oa.deadline).toLocaleString()} icon={<Clock size={10} />} />}
        </div>

        {(oa.status === 'in_progress' || oa.status === 'blocked' || (oa.progressNotes && oa.progressNotes.length > 0)) && (
          <div>
            <div className="mb-2 text-[9px] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]">Activity</div>
            <NotesSection notes={oa.progressNotes ?? []} actionId={oa.id} />
          </div>
        )}
      </div>

      {!showOutcomeForm && (
        <div className="border-t border-[color-mix(in_srgb,var(--border)_48%,transparent)] px-5 py-3">
          <LifecycleActions
            actionId={oa.id}
            status={oa.status}
            onShowOutcomeForm={() => setShowOutcomeForm(true)}
          />
        </div>
      )}
    </div>
  )
}

interface DashboardActionsPanelProps {
  projectId: string | null
  embedded?: boolean
  initialType?: TypeFilter
}

export default function DashboardActionsPanel({
  projectId,
  embedded = false,
  initialType = 'all',
}: DashboardActionsPanelProps) {
  const [typeFilter, setTypeFilter] = useState<TypeFilter>(initialType)
  const [completedExpanded, setCompletedExpanded] = useState(false)
  const [resolveTarget, setResolveTarget] = useState<EscalationView | null>(null)
  const [selectedActionId, setSelectedActionId] = useState<string | null>(null)

  useEffect(() => {
    setTypeFilter(initialType)
  }, [initialType])

  const {
    data: opActions,
    isLoading: oaLoading,
    isError: oaError,
    error: oaErrorObj,
    refetch: oaRefetch,
  } = usePendingOpActions(projectId)
  const {
    data: escalations,
    isLoading: escLoading,
    isError: escError,
    error: escErrorObj,
    refetch: escRefetch,
  } = usePendingEscalations(projectId)
  const { data: resolvedEscalations } = useResolvedEscalations(completedExpanded ? projectId : null)
  const { data: completedOAs } = useCompletedOpActions(completedExpanded ? projectId : null)

  if (!projectId) {
    return (
      <div className="flex h-full flex-col items-center justify-center gap-3 px-8 text-center">
        <ClipboardList size={36} className="text-[var(--text-3)] opacity-30" />
        <h2 className="m-0 text-base font-semibold text-[var(--text-1)]">No project selected</h2>
        <p className="m-0 text-sm text-[var(--text-3)]">Select a project to work with actions and escalations.</p>
      </div>
    )
  }

  if (selectedActionId) {
    return <ActionDetailInline actionId={selectedActionId} onBack={() => setSelectedActionId(null)} />
  }

  const isLoading = oaLoading || escLoading
  const isError = oaError || escError

  const allItems: ActionItem[] = [
    ...(opActions ?? []).map((oa): ActionItem => ({ kind: 'oa', item: oa })),
    ...(escalations ?? []).map((esc): ActionItem => ({ kind: 'escalation', item: esc })),
  ]

  const filteredItems = typeFilter === 'oa'
    ? allItems.filter((item) => item.kind === 'oa')
    : typeFilter === 'escalation'
      ? allItems.filter((item) => item.kind === 'escalation')
      : allItems

  const sorted = sortItems(filteredItems)
  const byTier = URGENCY_TIERS.reduce<Record<OperatorUrgency, ActionItem[]>>((acc, tier) => {
    acc[tier] = sorted.filter((item) => item.item.urgency === tier)
    return acc
  }, { critical: [], high: [], medium: [], low: [] })

  const historyCount = (resolvedEscalations?.length ?? 0) + (completedOAs?.length ?? 0)
  const totalCount = allItems.length

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={cn('shrink-0', embedded ? 'px-[var(--dashboard-context-gutter)] pt-5 pb-3 border-b border-[color-mix(in_srgb,var(--border)_56%,transparent)]' : 'px-6 pt-8 pb-2 max-w-4xl mx-auto w-full')}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h1 className="m-0 text-[var(--text-1)]" style={{ fontSize: embedded ? '19px' : '28px', fontWeight: 700, letterSpacing: embedded ? '-0.36px' : '-0.56px', lineHeight: embedded ? 1.18 : 1.14 }}>
              Actions
            </h1>
            <p className="m-0 mt-1 text-[var(--text-3)]" style={{ fontSize: embedded ? '12px' : '13px', letterSpacing: embedded ? '-0.12px' : '-0.08px', lineHeight: 1.45 }}>
              Work waiting on a person, plus escalations that need a clear decision.
            </p>
          </div>
          {!isLoading && totalCount > 0 && (
            <span className="text-[11px] font-semibold tabular-nums text-[var(--text-3)]">{totalCount} active</span>
          )}
        </div>
      </div>

      <div className={cn('shrink-0', embedded ? 'px-[var(--dashboard-context-gutter)] py-3 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)]' : 'px-6 py-2.5')}>
        <div className="flex items-center gap-2 overflow-x-auto">
          {TYPE_FILTERS.map((filter) => {
            const isActive = typeFilter === filter.value
            return (
              <button
                key={filter.value}
                onClick={() => setTypeFilter(filter.value)}
                aria-pressed={isActive}
                className={cn(
                  'inline-flex items-center gap-1.5 rounded-[var(--r-md)] border-none px-2.5 py-1 text-xs transition-colors cursor-pointer',
                  isActive
                    ? 'bg-[color-mix(in_srgb,var(--accent-dim)_18%,var(--bg-panel)_82%)] text-[var(--text-1)]'
                    : 'bg-transparent text-[var(--text-2)] hover:bg-[var(--bg-hover)]',
                )}
              >
                {filter.label}
              </button>
            )
          })}
        </div>
      </div>

      <div className={cn('flex-1 min-h-0 overflow-y-auto', embedded ? 'px-[var(--dashboard-context-gutter)] py-4' : 'px-6 pb-6')}>
        {isLoading && <ActionsSkeleton />}

        {!isLoading && isError && (
          <div className="flex flex-col items-center justify-center py-16 text-center gap-3">
            <AlertCircle size={36} className="text-[var(--red)] opacity-60" />
            <p className="text-sm text-[var(--text-2)]">Couldn&apos;t load actions. Check your connection.</p>
            {oaError && (
              <p className="text-xs text-[var(--text-3)]">{oaErrorObj instanceof Error ? oaErrorObj.message : 'Failed to load operator actions'}</p>
            )}
            {escError && (
              <p className="text-xs text-[var(--text-3)]">{escErrorObj instanceof Error ? escErrorObj.message : 'Failed to load escalations'}</p>
            )}
            <Button variant="secondary" size="sm" onClick={() => { void oaRefetch(); void escRefetch() }}>
              Retry
            </Button>
          </div>
        )}

        {!isLoading && !isError && sorted.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <ClipboardList size={40} className="text-[var(--text-3)] opacity-30 mb-4" />
            {typeFilter === 'all' ? (
              <>
                <h3 className="mb-1 text-base font-semibold text-[var(--text-1)]">Nothing needs your attention</h3>
                <p className="max-w-sm text-sm text-[var(--text-3)]">Your agents are handling things autonomously right now.</p>
              </>
            ) : (
              <>
                <h3 className="mb-1 text-base font-semibold text-[var(--text-1)]">No {typeFilter === 'oa' ? 'operator work' : 'escalations'} right now</h3>
                <Button variant="secondary" size="sm" className="mt-3" onClick={() => setTypeFilter('all')}>Show all</Button>
              </>
            )}
          </div>
        )}

        {!isLoading && !isError && sorted.length > 0 && (
          <div className="flex flex-col gap-6">
            {URGENCY_TIERS.map((tier) => {
              const tierItems = byTier[tier]
              if (tierItems.length === 0) return null
              return (
                <section key={tier}>
                  <div className="mb-3 flex items-center gap-2">
                    <span className="h-2 w-2 rounded-full shrink-0" style={{ backgroundColor: urgencyDotColor(tier) }} />
                    <span className="text-[10px] font-semibold uppercase tracking-[0.06em] text-[var(--text-2)]">{tier}</span>
                    <span className="text-[10px] tabular-nums text-[var(--text-3)]">{tierItems.length}</span>
                  </div>

                  <div className="dashboard-list-surface dashboard-list-surface-flat overflow-hidden">
                    {tierItems.map((item) => item.kind === 'oa' ? (
                      <OARow key={`oa-${item.item.id}`} oa={item.item} onClick={() => setSelectedActionId(item.item.id)} />
                    ) : (
                      <EscalationRow key={`esc-${item.item.id}`} esc={item.item} onClick={() => setResolveTarget(item.item)} />
                    ))}
                  </div>
                </section>
              )
            })}
          </div>
        )}

        {!isLoading && !isError && (
          <div className="pt-5">
            <button
              onClick={() => setCompletedExpanded((expanded) => !expanded)}
              className="flex items-center gap-2 bg-transparent border-none cursor-pointer p-0 text-xs text-[var(--text-3)] hover:text-[var(--text-2)] transition-colors"
            >
              {completedExpanded ? <ChevronDown size={13} /> : <ChevronRight size={13} />}
              <span>Completed &amp; resolved</span>
              {historyCount > 0 && <span className="text-[10px] tabular-nums">({historyCount})</span>}
            </button>
            {completedExpanded && (
              <div className="mt-3 dashboard-list-surface dashboard-list-surface-flat overflow-hidden">
                {historyCount === 0 ? (
                  <div className="px-3 py-4 text-xs text-[var(--text-3)]">Nothing completed or resolved yet.</div>
                ) : (
                  <>
                    {(completedOAs ?? []).map((oa) => (
                      <button
                        key={oa.id}
                        onClick={() => setSelectedActionId(oa.id)}
                        className={cn(
                          'dashboard-queue-row flex w-full items-center gap-3 bg-transparent px-3 py-2.5 text-left',
                          'border-0 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)] last:border-b-0 cursor-pointer font-[inherit]',
                        )}
                      >
                        <span className="dashboard-meta-chip dashboard-meta-chip-compact dashboard-meta-chip-muted shrink-0">
                          {oa.status === 'canceled' ? 'Canceled' : oa.outcomeStatus ?? 'Done'}
                        </span>
                        <div className="min-w-0 flex-1 text-[12px] font-medium text-[var(--text-2)] truncate">{oa.title}</div>
                        <div className="flex items-center gap-1 shrink-0 text-[var(--text-3)]">
                          <Clock size={10} />
                          <span className="text-[10px] tabular-nums">{timeAgo(oa.completedAt ?? oa.createdAt)}</span>
                        </div>
                      </button>
                    ))}
                    {(resolvedEscalations ?? []).map((esc) => (
                      <button
                        key={esc.id}
                        onClick={() => setResolveTarget(esc)}
                        className={cn(
                          'dashboard-queue-row flex w-full items-center gap-3 bg-transparent px-3 py-2.5 text-left',
                          'border-0 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)] last:border-b-0 cursor-pointer font-[inherit]',
                        )}
                      >
                        <span className="dashboard-meta-chip dashboard-meta-chip-compact dashboard-meta-chip-muted shrink-0">
                          {esc.status === 'resolved' ? esc.resolution ?? 'Resolved' : 'Canceled'}
                        </span>
                        <div className="min-w-0 flex-1 text-[12px] font-medium text-[var(--text-2)] truncate">{esc.title}</div>
                        <div className="flex items-center gap-1 shrink-0 text-[var(--text-3)]">
                          <Clock size={10} />
                          <span className="text-[10px] tabular-nums">{timeAgo(esc.resolvedAt ?? esc.createdAt)}</span>
                        </div>
                      </button>
                    ))}
                  </>
                )}
              </div>
            )}
          </div>
        )}
      </div>

      <ResolveDialog
        escalation={resolveTarget}
        initialResolution={null}
        projectId={projectId}
        onClose={() => setResolveTarget(null)}
        showNavLink={false}
      />
    </div>
  )
}
