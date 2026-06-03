import { useState } from 'react'
import { usePendingOpActions } from '../../hooks/useOpActions'
import { Skeleton } from '@/components/ui/skeleton'
import { ClipboardList, Clock, AlertCircle, User } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { OpActionView, OpActionStatus } from '../../lib/types'
import OpActionDetailPanel from './OpActionDetailPanel'

/* ── Status → badge mapping ──────────────────────────── */

function statusBadge(status: OpActionStatus): { tone: string; label: string } {
  switch (status) {
    case 'pending': return { tone: 'dashboard-meta-chip-warning', label: 'Pending' }
    case 'acknowledged': return { tone: 'dashboard-meta-chip-info', label: 'Seen' }
    case 'in_progress': return { tone: 'dashboard-meta-chip-accent', label: 'Working' }
    case 'pending_verification': return { tone: 'dashboard-meta-chip-warning', label: 'Verify' }
    case 'completed': return { tone: 'dashboard-meta-chip-success', label: 'Done' }
    case 'blocked': return { tone: 'dashboard-meta-chip-critical', label: 'Blocked' }
    case 'canceled': return { tone: 'dashboard-meta-chip-muted', label: 'Canceled' }
    default: return { tone: 'dashboard-meta-chip-muted', label: status }
  }
}

/* ── Time-ago helper ───────────────────────────────── */

function timeAgo(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return 'just now'
  const seconds = Math.floor(diffMs / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

/* ── Sort: urgency then creation ─────────────────────── */

function sortActions(actions: OpActionView[]): OpActionView[] {
  const urgencyOrder: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 }
  return [...actions].sort((a, b) => {
    const ua = urgencyOrder[a.urgency] ?? 4
    const ub = urgencyOrder[b.urgency] ?? 4
    if (ua !== ub) return ua - ub
    return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
  })
}

/* ── Lifecycle progress dots ─────────────────────────── */

function LifecycleDots({ status }: { status: OpActionStatus }) {
  const steps = ['pending', 'in_progress', 'completed'] as const
  const stepIndex = status === 'pending' || status === 'acknowledged' ? 0
    : status === 'in_progress' || status === 'blocked' ? 1
    : status === 'pending_verification' ? 1.5
    : status === 'completed' ? 2
    : -1 // canceled

  if (stepIndex === -1) return null

  return (
    <div className="flex items-center gap-0.5">
      {steps.map((_, i) => (
        <div
          key={i}
          className={cn(
            'w-1.5 h-1.5 rounded-full transition-colors',
            i < stepIndex ? 'bg-[var(--green)]'
              : i === Math.floor(stepIndex) ? 'bg-[var(--accent)]'
              : 'bg-[var(--border-strong)]',
          )}
        />
      ))}
    </div>
  )
}

/* ── Single OA row ────────────────────────────────────── */

function OARow({ oa, onSelect, isSelected, focusMode = false }: { oa: OpActionView; onSelect: () => void; isSelected: boolean; focusMode?: boolean }) {
  const status = statusBadge(oa.status)

  return (
    <button
      onClick={onSelect}
      className={cn(
        'dashboard-queue-row w-full text-left flex items-center gap-3 px-3 border-b border-[var(--border)] last:border-b-0',
        focusMode ? 'py-3.5' : 'py-2.5',
        'cursor-pointer bg-transparent border-0 font-[inherit]',
        isSelected && 'dashboard-queue-row-selected',
      )}
    >
      {/* Status badge */}
      <span className={cn('dashboard-meta-chip dashboard-meta-chip-compact shrink-0', status.tone)}>
        {status.label}
      </span>

      {/* Title + source */}
      <div className="flex-1 min-w-0">
        <div className="text-xs font-medium text-[var(--text-1)] truncate">{oa.title}</div>
        {oa.sourceMemberLabel && (
          <div className="flex items-center gap-1 mt-0.5">
            <User size={9} className="text-[var(--text-3)]" />
            <span className="text-[10px] text-[var(--text-3)] truncate">{oa.sourceMemberLabel}</span>
          </div>
        )}
      </div>

      {/* Lifecycle dots */}
      <LifecycleDots status={oa.status} />

      {/* Age */}
      <div className="flex items-center gap-1 shrink-0">
        <Clock size={10} className="text-[var(--text-3)]" />
        <span className="text-[10px] text-[var(--text-3)] tabular-nums">{timeAgo(oa.createdAt)}</span>
      </div>
    </button>
  )
}

/* ── Loading skeleton ──────────────────────────────── */

function QueueSkeleton() {
  return (
    <div className="dashboard-section">
      <div className="flex items-center gap-2 mb-3">
        <Skeleton className="h-4 w-4 rounded-full" />
        <Skeleton className="h-4 w-48" />
      </div>
      <div className="dashboard-list-surface">
        {[1, 2].map(i => (
          <div key={i} className="flex items-center gap-3 px-3 py-2.5 border-b border-[var(--border)] last:border-b-0">
            <Skeleton className="h-4 w-16 rounded-full" />
            <Skeleton className="h-4 flex-1" />
            <Skeleton className="h-4 w-12" />
          </div>
        ))}
      </div>
    </div>
  )
}

/* ── Main exported component ───────────────────────── */

export default function OpActionQueue({ projectId, initialSelectedId, focusMode = false }: { projectId: string | null; initialSelectedId?: string | null; focusMode?: boolean }) {
  const { data: actions, isLoading, isError, error } = usePendingOpActions(projectId)
  // Support ?action=:id query param on mount (deep-link from notifications)
  const [selectedId, setSelectedId] = useState<string | null>(
    () => new URLSearchParams(window.location.search).get('action') ?? initialSelectedId ?? null,
  )

  if (!projectId) return null
  if (isLoading) return <QueueSkeleton />

  if (isError) {
    return (
      <div className="dashboard-section flex items-center gap-2 px-1 py-3 text-xs text-[var(--red)]">
        <AlertCircle size={14} />
        <span>Failed to load operator actions: {error instanceof Error ? error.message : 'Unknown error'}</span>
      </div>
    )
  }

  const sorted = sortActions(actions ?? [])

  return (
    <>
      {sorted.length > 0 && (
        <section className={cn('dashboard-section', focusMode && 'max-w-none')}>
          <div className="dashboard-section-heading mb-3">
            <div className="dashboard-section-heading-main">
              <div className="flex items-center gap-2">
                <ClipboardList size={14} className="text-[var(--accent)]" />
                <span className={cn('dashboard-section-title', focusMode ? 'text-base' : 'text-sm')}>
                  Outside agen8
                </span>
              </div>
              <p className="dashboard-section-caption">Work waiting on an email, a call, or a step outside the system.</p>
            </div>
            <div className="dashboard-section-meta">
              <span className="dashboard-section-counter text-[var(--accent)]">
                {sorted.length} active
              </span>
            </div>
          </div>
          <div className={cn(
            'dashboard-list-surface dashboard-list-surface-flat dashboard-side-surface',
            focusMode ? 'max-w-none overflow-hidden' : sorted.length > 8 ? 'max-h-[360px] overflow-y-auto overflow-x-hidden' : 'overflow-hidden',
          )}>
            {sorted.map(oa => (
              <OARow
                key={oa.id}
                oa={oa}
                isSelected={selectedId === oa.id}
                onSelect={() => setSelectedId(oa.id)}
                focusMode={focusMode}
              />
            ))}
          </div>
        </section>
      )}

      <OpActionDetailPanel
        actionId={selectedId}
        projectId={projectId}
        onClose={() => setSelectedId(null)}
      />
    </>
  )
}
