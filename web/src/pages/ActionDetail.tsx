import { useState } from 'react'
import { Link, useLocation } from 'wouter'
import { useOpAction } from '../hooks/useOpActions'
import { actionsPanelLink } from '../lib/routing'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  Breadcrumb,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from '@/components/ui/breadcrumb'
import { AlertCircle, ArrowLeft, Clock, User, FileText } from 'lucide-react'
import { statusConfig, timeAgo } from '../components/actions/opActionUtils'
import {
  NotesSection,
  OutcomeForm,
  LifecycleActions,
} from '../components/actions/OpActionContent'
import { cn } from '@/lib/utils'
import type { OpActionNote, OpActionStatus } from '../lib/types'

/* ── Unified activity timeline ───────────────────── */

type TimelineEntry =
  | { kind: 'system'; label: string; at: string }
  | { kind: 'note'; text: string; at: string }

function buildTimeline(oa: {
  createdAt: string
  acknowledgedAt?: string
  startedAt?: string
  verifiedAt?: string
  completedAt?: string
  status: OpActionStatus
  progressNotes?: OpActionNote[]
}): TimelineEntry[] {
  const entries: TimelineEntry[] = []
  entries.push({ kind: 'system', label: 'Action created', at: oa.createdAt })
  if (oa.acknowledgedAt) entries.push({ kind: 'system', label: 'Acknowledged', at: oa.acknowledgedAt })
  if (oa.startedAt) entries.push({ kind: 'system', label: 'Work started', at: oa.startedAt })
  if (oa.verifiedAt) entries.push({ kind: 'system', label: 'Verified', at: oa.verifiedAt })
  if (oa.completedAt) {
    entries.push({ kind: 'system', label: oa.status === 'canceled' ? 'Action canceled' : 'Action completed', at: oa.completedAt })
  }
  for (const note of oa.progressNotes ?? []) {
    entries.push({ kind: 'note', text: note.text, at: note.createdAt })
  }
  return entries.sort((a, b) => a.at.localeCompare(b.at))
}

/* ── Loading skeleton ──────────────────────────────── */

function ActionDetailSkeleton() {
  return (
    <div className="flex flex-col h-full">
      <div className="px-6 pt-4 pb-4 border-b border-[var(--border)] shrink-0">
        <Skeleton className="h-4 w-56 mb-3" />
        <Skeleton className="h-7 w-2/3 mb-2" />
        <Skeleton className="h-4 w-72" />
      </div>
      <div className="flex-1 overflow-y-auto">
        <div className="max-w-4xl mx-auto flex gap-0">
          <div className="flex-1 px-6 py-5 space-y-6">
            <Skeleton className="h-16 w-full" />
            <div className="border-t border-[var(--border)]" />
            <div className="space-y-4">
              {[1, 2, 3].map(i => <Skeleton key={i} className="h-8 w-full" />)}
            </div>
          </div>
          <div className="w-48 shrink-0 border-l border-[var(--border)] px-5 py-5 space-y-4">
            {[1, 2, 3, 4, 5].map(i => <Skeleton key={i} className="h-8 w-full" />)}
          </div>
        </div>
      </div>
    </div>
  )
}

/* ── Main page ───────────────────────────────────────── */

export default function ActionDetail({ params }: { params: { projectId: string; actionId: string } }) {
  const projectId = decodeURIComponent(params.projectId)
  const actionId = decodeURIComponent(params.actionId)
  const [, navigate] = useLocation()
  const [showOutcomeForm, setShowOutcomeForm] = useState(false)
  const [showAllActivity, setShowAllActivity] = useState(false)

  const { data: oa, isLoading, isError } = useOpAction(actionId)

  const actionsLink = actionsPanelLink(projectId)
  const dashboardLink = `/project/${encodeURIComponent(projectId)}/dashboard`

  if (isLoading) return <ActionDetailSkeleton />

  if (isError || !oa) {
    return (
      <div className="flex flex-col h-full">
        <div className="px-6 pt-5 pb-3 border-b border-[var(--border)]">
          <Breadcrumb>
            <BreadcrumbList>
              <BreadcrumbItem><BreadcrumbLink asChild><Link to={dashboardLink}>Dashboard</Link></BreadcrumbLink></BreadcrumbItem>
              <BreadcrumbSeparator />
              <BreadcrumbItem><BreadcrumbLink asChild><Link to={actionsLink}>Actions</Link></BreadcrumbLink></BreadcrumbItem>
            </BreadcrumbList>
          </Breadcrumb>
        </div>
        <div className="flex-1 flex flex-col items-center justify-center gap-4 px-6">
          <AlertCircle size={32} className="text-[var(--text-3)]" />
          <p className="text-sm text-[var(--text-2)] text-center">This action doesn&apos;t exist or has been removed.</p>
          <Button variant="outline" size="sm" onClick={() => navigate(actionsLink)}>
            <ArrowLeft size={12} className="mr-1.5" /> Back to Actions
          </Button>
        </div>
      </div>
    )
  }

  const sc = statusConfig(oa.status)
  const isTerminal = oa.status === 'completed' || oa.status === 'canceled'
  const timeline = buildTimeline(oa)

  const ACTIVITY_VISIBLE = 6
  const hiddenCount = Math.max(0, timeline.length - ACTIVITY_VISIBLE)
  const visibleTimeline = showAllActivity || hiddenCount === 0 ? timeline : timeline.slice(-ACTIVITY_VISIBLE)

  return (
    <div className="flex flex-col h-full">

      {/* ── Header ──────────────────────────────────────── */}
      <header className="px-6 pt-4 pb-3 border-b border-[var(--border)] shrink-0">
        <Breadcrumb className="mb-3">
          <BreadcrumbList>
            <BreadcrumbItem>
              <BreadcrumbLink asChild><Link to={dashboardLink}>Dashboard</Link></BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbLink asChild><Link to={actionsLink}>Actions</Link></BreadcrumbLink>
            </BreadcrumbItem>
            <BreadcrumbSeparator />
            <BreadcrumbItem>
              <BreadcrumbPage className="max-w-[260px] truncate">{oa.title}</BreadcrumbPage>
            </BreadcrumbItem>
          </BreadcrumbList>
        </Breadcrumb>

        <div className="flex items-start gap-4">
          <div className="flex-1 min-w-0">
            <h1 className="text-xl font-bold text-[var(--text-1)] tracking-[-0.02em] leading-tight mb-2">
              {oa.title}
            </h1>
            {/* Compact metadata bar */}
            <div className="flex items-center gap-1.5 flex-wrap text-xs text-[var(--text-3)]">
              <span
                className="text-[10px] font-semibold px-2 py-0.5 rounded-full shrink-0"
                style={{ color: sc.color, background: sc.bg }}
              >
                {sc.label}
              </span>
              {oa.category && (
                <>
                  <span className="text-[var(--border-strong)]">·</span>
                  <span>{oa.category}</span>
                </>
              )}
              {oa.sourceMemberLabel && (
                <>
                  <span className="text-[var(--border-strong)]">·</span>
                  <span className="flex items-center gap-1">
                    <User size={10} className="shrink-0" />{oa.sourceMemberLabel}
                  </span>
                </>
              )}
              <span className="text-[var(--border-strong)]">·</span>
              <span className="flex items-center gap-1">
                <Clock size={10} className="shrink-0" />{timeAgo(oa.createdAt)}
              </span>
              {oa.urgency === 'critical' && <Badge variant="danger" className="text-[10px] px-1.5 py-0 ml-0.5">Critical</Badge>}
              {oa.urgency === 'high' && <Badge variant="warning" className="text-[10px] px-1.5 py-0 ml-0.5">High</Badge>}
              {oa.blocking && <Badge variant="secondary" className="text-[10px] px-1.5 py-0 ml-0.5">Blocking agent</Badge>}
            </div>
          </div>

          {/* Lifecycle actions live in the header, not the content */}
          {!isTerminal && !showOutcomeForm && (
            <div className="shrink-0 pt-0.5">
              <LifecycleActions
                actionId={oa.id}
                status={oa.status}
                onShowOutcomeForm={() => setShowOutcomeForm(true)}
              />
            </div>
          )}
        </div>
      </header>

      {/* ── Body ─────────────────────────────────────────── */}
      <div className="flex-1 overflow-y-auto min-h-0">
        <div className="max-w-4xl mx-auto flex items-start min-h-full">

          {/* ── Main content ── */}
          <div className="flex-1 min-w-0 px-6 py-5 space-y-6">

            {/* Outcome form (when completing) */}
            {showOutcomeForm && (
              <OutcomeForm actionId={oa.id} onCancel={() => setShowOutcomeForm(false)} />
            )}

            {/* Outcome result banner (terminal) */}
            {oa.outcomeSummary && (
              <div className={cn(
                'rounded-[var(--r-md)] px-4 py-3 border',
                oa.outcomeStatus === 'failed'
                  ? 'bg-[var(--red-dim)] border-[var(--red)]/20'
                  : oa.outcomeStatus === 'partial'
                    ? 'bg-[var(--amber-dim)] border-[var(--amber)]/20'
                    : 'bg-[var(--green-dim)] border-[var(--green)]/20',
              )}>
                <div className={cn(
                  'text-[10px] font-semibold uppercase tracking-[0.06em] mb-1',
                  oa.outcomeStatus === 'failed' ? 'text-[var(--red)]'
                    : oa.outcomeStatus === 'partial' ? 'text-[var(--amber)]'
                    : 'text-[var(--green)]',
                )}>
                  {oa.outcomeStatus ?? 'completed'}
                </div>
                <div className="text-sm text-[var(--text-2)]">{oa.outcomeSummary}</div>
                {oa.outcomePairs && Object.keys(oa.outcomePairs).length > 0 && (
                  <div className="mt-3 pt-3 border-t border-[var(--border)] grid grid-cols-2 gap-x-4 gap-y-1.5">
                    {Object.entries(oa.outcomePairs).map(([k, v]) => (
                      <div key={k}>
                        <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em]">{k}</div>
                        <div className="text-xs text-[var(--text-1)] font-medium truncate">{v}</div>
                      </div>
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Description — flat, document-like body */}
            <div>
              <div className="text-[10px] font-semibold text-[var(--text-3)] uppercase tracking-[0.06em] mb-2.5">
                Description
              </div>
              <div className="text-sm text-[var(--text-2)] leading-relaxed whitespace-pre-wrap">
                {oa.description
                  ? oa.description
                  : <span className="italic text-[var(--text-3)]">No description provided.</span>}
              </div>
            </div>

            <div className="border-t border-[var(--border)]" />

            {/* Activity — unified event + note timeline */}
            <div>
              <div className="text-[10px] font-semibold text-[var(--text-3)] uppercase tracking-[0.06em] mb-4">
                Activity
              </div>

              <div className="space-y-0">
                {/* "N earlier events" expander — sits above the visible entries */}
                {hiddenCount > 0 && (
                  <div className="flex gap-3 pb-4">
                    <div className="flex flex-col items-center shrink-0 w-[10px]">
                      <div className="w-px flex-1 bg-[var(--border)] min-h-[12px]" />
                      <div className="text-[8px] text-[var(--border-strong)] leading-none my-0.5">•••</div>
                      <div className="w-px flex-1 bg-[var(--border)] min-h-[12px]" />
                    </div>
                    <button
                      onClick={() => setShowAllActivity(v => !v)}
                      className="text-xs text-[var(--text-3)] hover:text-[var(--text-2)] self-center transition-colors"
                    >
                      {showAllActivity
                        ? 'Collapse earlier events'
                        : `${hiddenCount} earlier event${hiddenCount !== 1 ? 's' : ''}`}
                    </button>
                  </div>
                )}

                {visibleTimeline.map((entry, i) => {
                  const isLast = i === visibleTimeline.length - 1
                  return (
                    <div key={i} className="flex gap-3">
                      {/* Node + connector line */}
                      <div className="flex flex-col items-center shrink-0">
                        {entry.kind === 'system' ? (
                          <div className="w-1.5 h-1.5 rounded-full bg-[var(--border-strong)] mt-1.5 shrink-0" />
                        ) : (
                          <div className="w-2.5 h-2.5 rounded-full border-2 border-[var(--accent)] bg-[var(--bg)] mt-1 shrink-0" />
                        )}
                        {!isLast && (
                          <div className="w-px flex-1 bg-[var(--border)] mt-1.5 min-h-[20px]" />
                        )}
                      </div>

                      {/* Entry content */}
                      <div className={cn('min-w-0', isLast ? 'pb-0' : 'pb-4')}>
                        {entry.kind === 'system' ? (
                          <div className="flex items-baseline gap-1.5">
                            <span className="text-xs text-[var(--text-3)]">{entry.label}</span>
                            <span className="text-[10px] text-[var(--border-strong)] tabular-nums">{timeAgo(entry.at)}</span>
                          </div>
                        ) : (
                          <>
                            <div className="text-sm text-[var(--text-2)] leading-snug">{entry.text}</div>
                            <div className="text-[10px] text-[var(--text-3)] tabular-nums mt-0.5">{timeAgo(entry.at)}</div>
                          </>
                        )}
                      </div>
                    </div>
                  )
                })}
              </div>

              {/* Add note input — not shown for terminal actions */}
              {!isTerminal && (
                <div className="mt-5 pl-5">
                  <NotesSection notes={[]} actionId={oa.id} inputOnly />
                </div>
              )}
            </div>
          </div>

          {/* ── Right sidebar: pure metadata ── */}
          <aside className="w-48 shrink-0 border-l border-[var(--border)] px-5 py-5">
            <div className="space-y-4">
              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1.5">Status</div>
                <span
                  className="text-[10px] font-semibold px-2 py-0.5 rounded-full"
                  style={{ color: sc.color, background: sc.bg }}
                >
                  {sc.label}
                </span>
              </div>

              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Priority</div>
                <div className="text-xs text-[var(--text-2)] capitalize">{oa.urgency}</div>
              </div>

              {oa.category && (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Category</div>
                  <div className="text-xs text-[var(--text-2)]">{oa.category}</div>
                </div>
              )}

              {oa.sourceMemberLabel && (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Requested by</div>
                  <div className="text-xs text-[var(--text-2)] flex items-center gap-1">
                    <User size={10} className="text-[var(--text-3)] shrink-0" />
                    {oa.sourceMemberLabel}
                  </div>
                </div>
              )}

              {oa.taskRef && (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Task</div>
                  <div className="text-xs text-[var(--text-2)] flex items-center gap-1">
                    <FileText size={10} className="text-[var(--text-3)] shrink-0" />
                    {oa.taskRef}
                  </div>
                </div>
              )}

              {oa.deadline && (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Deadline</div>
                  <div className="text-xs text-[var(--text-2)] flex items-center gap-1">
                    <Clock size={10} className="text-[var(--text-3)] shrink-0" />
                    {new Date(oa.deadline).toLocaleString()}
                  </div>
                </div>
              )}

              <div className="h-px bg-[var(--border)]" />

              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Created</div>
                <div className="text-xs text-[var(--text-2)]">{timeAgo(oa.createdAt)}</div>
              </div>

              {oa.startedAt && (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Started</div>
                  <div className="text-xs text-[var(--text-2)]">{timeAgo(oa.startedAt)}</div>
                </div>
              )}

              {oa.completedAt && (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Completed</div>
                  <div className="text-xs text-[var(--text-2)]">{timeAgo(oa.completedAt)}</div>
                </div>
              )}
            </div>
          </aside>

        </div>
      </div>
    </div>
  )
}
