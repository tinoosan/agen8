import { useState, useEffect, useRef, useCallback } from 'react'
import { useLocation } from 'wouter'
import { toast } from 'sonner'
import { usePendingEscalations, useResolveEscalation } from '../../hooks/useEscalations'
import { actionsPanelLink } from '../../lib/routing'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { Textarea } from '@/components/ui/textarea'
import { Input } from '@/components/ui/input'
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
  DialogDescription,
  DialogFooter,
} from '@/components/ui/dialog'
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select'
import { Checkbox } from '@/components/ui/checkbox'
import { AlertTriangle, CheckCircle2, XCircle, AlertCircle, Clock, ChevronRight, ExternalLink, Check, MoreHorizontal } from 'lucide-react'
import { cn } from '@/lib/utils'
import type { EscalationView, OperatorUrgency, OperatorCategory, EscalationResolution } from '../../lib/types'

/* ── Urgency → badge mapping ───────────────────────── */

function urgencyBadge(urgency: OperatorUrgency): { tone: string; label: string } {
  switch (urgency) {
    case 'critical': return { tone: 'dashboard-meta-chip-critical', label: 'Critical' }
    case 'high': return { tone: 'dashboard-meta-chip-warning', label: 'High' }
    case 'medium': return { tone: 'dashboard-meta-chip-info', label: 'Medium' }
    case 'low': default: return { tone: 'dashboard-meta-chip-muted', label: 'Low' }
  }
}

/* ── Category labels ──────────────────────────────── */

const categoryLabels: Record<string, string> = {
  financial: 'Financial', legal: 'Legal', content: 'Content', code: 'Code',
  general: 'General', physical: 'Physical', communication: 'Comms', administrative: 'Admin',
}

function categoryLabel(cat: OperatorCategory | string): string {
  return categoryLabels[cat] ?? cat
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

function mutationErrorMessage(error: unknown): string {
  if (error instanceof Error && error.message.trim()) {
    return error.message
  }
  if (typeof error === 'string' && error.trim()) {
    return error
  }
  return 'Unknown error'
}

/* ── Sort: critical first, then by creation time ───── */

function sortEscalations(items: EscalationView[]): EscalationView[] {
  const urgencyOrder: Record<string, number> = { critical: 0, high: 1, medium: 2, low: 3 }
  return [...items].sort((a, b) => {
    const ua = urgencyOrder[a.urgency] ?? 4
    const ub = urgencyOrder[b.urgency] ?? 4
    if (ua !== ub) return ua - ub
    return new Date(a.createdAt).getTime() - new Date(b.createdAt).getTime()
  })
}

function resolutionButtonClass(resolution: EscalationResolution): string {
  switch (resolution) {
    case 'approve':
      return 'text-[var(--green)] hover:text-[var(--green)] hover:bg-[var(--green-dim)]'
    case 'reject':
      return 'text-[var(--red)] hover:text-[var(--red)] hover:bg-[var(--red-dim)]'
    default:
      return 'text-[var(--text-2)]'
  }
}

/* ── Resolution dialog ─────────────────────────────── */

const RESOLUTION_OPTIONS: { value: EscalationResolution; label: string; description: string }[] = [
  { value: 'approve', label: 'Approve', description: 'Proceed as recommended' },
  { value: 'reject', label: 'Reject', description: 'Stop — do not proceed' },
  { value: 'redirect', label: 'Redirect', description: 'Change approach — provide feedback' },
  { value: 'defer', label: 'Defer', description: 'Postpone — task stays blocked' },
  { value: 'delegate', label: 'Delegate', description: 'Reassign to another member or space' },
]

interface ResolveDialogProps {
  escalation: EscalationView | null
  initialResolution: EscalationResolution | null
  projectId: string | null
  onClose: () => void
  showNavLink?: boolean
}

function resolutionChipStyle(res: EscalationResolution | string): { color: string; bg: string } {
  switch (res) {
    case 'approve': return { color: 'var(--green)', bg: 'var(--green-dim)' }
    case 'reject': return { color: 'var(--red)', bg: 'var(--red-dim)' }
    case 'redirect': return { color: 'var(--amber)', bg: 'var(--amber-dim)' }
    default: return { color: 'var(--text-2)', bg: 'var(--bg-surface)' }
  }
}

export function ResolveDialog({ escalation, initialResolution, projectId, onClose, showNavLink = true }: ResolveDialogProps) {
  const [resolution, setResolution] = useState<EscalationResolution | ''>(initialResolution ?? '')
  const [note, setNote] = useState('')
  const [delegatedTo, setDelegatedTo] = useState('')
  const resolve = useResolveEscalation()
  const [, navigate] = useLocation()

  const isResolved = escalation?.status === 'resolved' || escalation?.status === 'canceled' || escalation?.status === 'expired'

  const handleSubmit = () => {
    if (!escalation || !resolution) return
    resolve.mutate(
      {
        escalationId: escalation.id,
        resolution,
        resolutionNote: note || undefined,
        resolvedBy: 'operator',
        delegatedTo: resolution === 'delegate' ? delegatedTo : undefined,
      },
      { onSuccess: () => { setNote(''); setDelegatedTo(''); onClose() } },
    )
  }

  const resolutionColor = resolution === 'approve' ? 'bg-[var(--green)] hover:bg-[var(--green)]/90'
    : resolution === 'reject' ? 'bg-[var(--red)] hover:bg-[var(--red)]/90'
    : 'bg-[var(--accent)] hover:bg-[var(--accent)]/90'

  return (
    <Dialog open={!!escalation} onOpenChange={(open) => { if (!open) { setNote(''); setDelegatedTo(''); onClose() } }}>
      <DialogContent className="dashboard-dialog-content max-w-md">
        <DialogHeader>
          <DialogTitle className="text-sm font-semibold">
            {isResolved ? 'Escalation' : 'Resolve Escalation'}
          </DialogTitle>
          <DialogDescription className="text-xs text-[var(--text-3)]">
            {escalation?.title}
          </DialogDescription>
        </DialogHeader>

        {isResolved ? (
          /* ── Read-only view for resolved/canceled escalations ── */
          <div className="space-y-4 py-1">
            {/* Resolution decision */}
            {escalation?.resolution && (() => {
              const chip = resolutionChipStyle(escalation.resolution)
              const label = RESOLUTION_OPTIONS.find(o => o.value === escalation.resolution)?.label ?? escalation.resolution
              return (
                <div>
                  <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1.5">Decision</div>
                  <span
                    className="text-xs font-semibold px-2.5 py-1 rounded-full"
                    style={{ color: chip.color, background: chip.bg }}
                  >
                    {label}
                  </span>
                </div>
              )
            })()}

            {/* Description */}
            {escalation?.description && (
              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1.5">Description</div>
                <p className="text-xs text-[var(--text-2)] leading-relaxed">{escalation.description}</p>
              </div>
            )}

            {/* Delegated to */}
            {escalation?.delegatedTo && (
              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Delegated to</div>
                <p className="text-xs text-[var(--text-2)]">{escalation.delegatedTo}</p>
              </div>
            )}

            {/* Resolution note */}
            {escalation?.resolutionNote && (
              <div>
                <div className="text-[9px] font-semibold text-[var(--text-3)] uppercase tracking-[0.04em] mb-1">Note</div>
                <p className="text-xs text-[var(--text-2)] leading-relaxed">{escalation.resolutionNote}</p>
              </div>
            )}

            {/* Who + when */}
            <div className="flex items-center gap-3 pt-1 border-t border-[var(--border)]">
              {escalation?.resolvedBy && (
                <span className="text-[10px] text-[var(--text-3)]">by {escalation.resolvedBy}</span>
              )}
              {escalation?.resolvedAt && (
                <span className="text-[10px] text-[var(--text-3)] tabular-nums flex items-center gap-1">
                  <Clock size={9} /> {timeAgo(escalation.resolvedAt)}
                </span>
              )}
            </div>
          </div>
        ) : (
          /* ── Active resolve form ── */
          <div className="space-y-3">
            <Select value={resolution} onValueChange={(v) => setResolution(v as EscalationResolution)}>
              <SelectTrigger className="text-xs">
                <SelectValue placeholder="Select resolution..." />
              </SelectTrigger>
              <SelectContent>
                {RESOLUTION_OPTIONS.map(opt => (
                  <SelectItem key={opt.value} value={opt.value} className="text-xs">
                    <span className="font-medium">{opt.label}</span>
                    <span className="text-[var(--text-3)] ml-2">{opt.description}</span>
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {resolution === 'delegate' && (
              <Input
                placeholder="Delegate to a member or space..."
                value={delegatedTo}
                onChange={(e) => setDelegatedTo(e.target.value)}
                className="text-xs"
              />
            )}

            <Textarea
              placeholder="Resolution note (optional)"
              value={note}
              onChange={(e) => setNote(e.target.value)}
              className="min-h-[60px] text-xs"
            />
          </div>
        )}

        <DialogFooter>
          {showNavLink && projectId && (
            <button
              onClick={() => { onClose(); navigate(actionsPanelLink(projectId, 'escalation')) }}
              className="flex items-center gap-1 text-xs text-[var(--accent)] hover:underline mr-auto bg-transparent border-0 p-0 cursor-pointer"
            >
              Open in Actions <ExternalLink size={10} className="ml-0.5" />
            </button>
          )}
          {isResolved ? (
            <Button variant="outline" size="sm" onClick={onClose}>Close</Button>
          ) : (
            <>
              <Button variant="outline" size="sm" onClick={() => { setNote(''); setDelegatedTo(''); onClose() }}>
                Cancel
              </Button>
              <Button
                size="sm"
                onClick={handleSubmit}
                disabled={resolve.isPending || !resolution || (resolution === 'delegate' && !delegatedTo.trim())}
                className={cn('text-white', resolutionColor)}
              >
                {resolve.isPending ? 'Resolving...' : resolution ? RESOLUTION_OPTIONS.find(o => o.value === resolution)?.label ?? 'Resolve' : 'Select resolution'}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

/* ── Single escalation row ────────────────────────────── */

function EscalationRow({
  esc, onResolve, onQuickApprove, initialExpanded = false, focusMode = false,
}: {
  esc: EscalationView
  onResolve: (esc: EscalationView, resolution: EscalationResolution) => void
  onQuickApprove: (id: string) => void
  initialExpanded?: boolean
  focusMode?: boolean
}) {
  const [expanded, setExpanded] = useState(initialExpanded)
  const rowRef = useRef<HTMLDivElement>(null)

  // Scroll into view if initially expanded via deep link
  useEffect(() => {
    if (initialExpanded && rowRef.current) {
      rowRef.current.scrollIntoView({ behavior: 'smooth', block: 'nearest' })
    }
  }, [initialExpanded])
  const urgency = urgencyBadge(esc.urgency as OperatorUrgency)
  const rowPadding = focusMode ? 'py-3.5' : 'py-2.5'

  return (
    <div
      ref={rowRef}
      className="dashboard-queue-row group border-b border-[var(--border)] last:border-b-0"
    >
      <div className={cn('flex items-center gap-2.5 px-3', rowPadding)}>
        {/* Expand chevron */}
        <button
          onClick={() => setExpanded(e => !e)}
          className="shrink-0 text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors bg-transparent border-0 p-0 cursor-pointer"
        >
          <ChevronRight
            size={11}
            className={cn('transition-transform duration-150', expanded && 'rotate-90')}
          />
        </button>

        {/* Urgency badge */}
        <span className={cn('dashboard-meta-chip dashboard-meta-chip-compact shrink-0', urgency.tone)}>
          {urgency.label}
        </span>

        {/* Title + category + recommendation */}
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 min-w-0">
            <span className="text-xs font-medium text-[var(--text-1)] truncate">{esc.title}</span>
            <span className="dashboard-inline-label shrink-0">
              {categoryLabel(esc.category)}
            </span>
          </div>
          {!expanded && esc.recommendation && (
            <div className="text-[10px] text-[var(--text-3)] truncate mt-0.5" title={esc.recommendation}>
              Rec: {esc.recommendation}
              {esc.confidence != null && esc.confidence > 0 && (
                <span className="ml-1">({Math.round(esc.confidence * 100)}%)</span>
              )}
            </div>
          )}
        </div>

        {/* Age */}
        <div className="flex items-center gap-1 shrink-0">
          <Clock size={10} className="text-[var(--text-3)]" />
          <span className="text-[10px] text-[var(--text-3)] tabular-nums">{timeAgo(esc.createdAt)}</span>
        </div>

        {/* Inline quick-action buttons — always visible, full opacity on row hover */}
        {!expanded && (
          <div className="flex items-center gap-0.5 shrink-0 opacity-55 group-hover:opacity-100 transition-opacity">
            <button
              onClick={(e) => { e.stopPropagation(); onQuickApprove(esc.id) }}
              title="Quick approve"
              className="dashboard-queue-icon-button text-[var(--green)] hover:text-[var(--green)] hover:bg-[var(--green-dim)]"
            >
              <Check size={12} />
            </button>
            <button
              onClick={(e) => { e.stopPropagation(); onResolve(esc, 'reject') }}
              title="Resolve / more options"
              className="dashboard-queue-icon-button text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)]"
            >
              <MoreHorizontal size={12} />
            </button>
          </div>
        )}

      </div>

      {/* Expanded details */}
      {expanded && (
        <div className="animate-fade-in px-4 pb-3 pl-10">
          <div className="dashboard-queue-detail p-3 text-xs text-[var(--text-2)] leading-relaxed">
            {esc.description}
            {esc.sourceMemberLabel && (
              <div className="mt-2 text-[10px] text-[var(--text-3)]">
                Escalated by: <span className="font-medium text-[var(--text-2)]">{esc.sourceMemberLabel}</span>
              </div>
            )}
            {esc.deadline && (
              <div className="mt-1 text-[10px] text-[var(--text-3)]">
                Deadline: <span className="font-medium text-[var(--amber)]">{new Date(esc.deadline).toLocaleString()}</span>
              </div>
            )}
            <div className="mt-2 flex flex-wrap gap-1.5">
              {RESOLUTION_OPTIONS.map(({ value, label }) => (
                <Button
                  key={value}
                  variant="outline"
                  size="xs"
                  className={cn('dashboard-action-button text-[10px]', resolutionButtonClass(value))}
                  onClick={() => onResolve(esc, value)}
                >
                  {value === 'approve' && <CheckCircle2 size={11} className="mr-1" />}
                  {value === 'reject' && <XCircle size={11} className="mr-1" />}
                  {label}
                </Button>
              ))}
            </div>
          </div>
        </div>
      )}
    </div>
  )
}

/* ── Loading skeleton ──────────────────────────────── */

function QueueSkeleton() {
  return (
    <div className="dashboard-section">
      <div className="flex items-center gap-2 mb-3">
        <Skeleton className="h-4 w-4 rounded-full" />
        <Skeleton className="h-4 w-40" />
      </div>
      <div className="dashboard-list-surface">
        {[1, 2, 3].map(i => (
          <div key={i} className="flex items-center gap-3 px-3 py-2.5 border-b border-[var(--border)] last:border-b-0">
            <Skeleton className="h-4 w-14 rounded-full" />
            <Skeleton className="h-4 flex-1" />
            <Skeleton className="h-4 w-12" />
            <Skeleton className="h-5 w-12 rounded-[var(--r-sm)]" />
          </div>
        ))}
      </div>
    </div>
  )
}

/* ── Main exported component ───────────────────────── */

export default function EscalationQueue({ projectId, initialSelectedId, focusMode = false }: { projectId: string | null; initialSelectedId?: string | null; focusMode?: boolean }) {
  const { data: escalations, isLoading, isError, error } = usePendingEscalations(projectId)
  const [resolveTarget, setResolveTarget] = useState<{ esc: EscalationView; resolution: EscalationResolution } | null>(null)
  const [selectedIds, setSelectedIds] = useState<Set<string>>(new Set())
  const resolve = useResolveEscalation()

  const handleQuickApprove = useCallback((id: string) => {
    resolve.mutate(
      { escalationId: id, resolution: 'approve', resolvedBy: 'operator' },
      {
        onSuccess: () => toast.success('Escalation approved'),
        onError: (error) => toast.error(`Failed to approve escalation: ${mutationErrorMessage(error)}`),
      },
    )
  }, [resolve])

  const toggleSelection = useCallback((id: string) => {
    setSelectedIds(prev => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id); else next.add(id)
      return next
    })
  }, [])

  const toggleAll = useCallback(() => {
    if (!escalations) return
    setSelectedIds(prev => prev.size === escalations.length ? new Set() : new Set(escalations.map(e => e.id)))
  }, [escalations])

  const handleBulkApprove = useCallback(async () => {
    if (selectedIds.size === 0) return
    const ids = Array.from(selectedIds)
    let approved = 0
    const failures = new Map<string, string>()

    for (const id of ids) {
      try {
        await resolve.mutateAsync({ escalationId: id, resolution: 'approve', resolvedBy: 'operator' })
        approved++
      } catch (error) {
        failures.set(id, mutationErrorMessage(error))
      }
    }

    if (failures.size > 0) {
      setSelectedIds(new Set(failures.keys()))
      const firstError = Array.from(failures.values())[0]
      if (approved > 0) {
        toast.error(`Approved ${approved} escalation${approved !== 1 ? 's' : ''}; ${failures.size} failed: ${firstError}`)
      } else {
        toast.error(`Failed to approve ${failures.size} escalation${failures.size !== 1 ? 's' : ''}: ${firstError}`)
      }
      return
    }

    setSelectedIds(new Set())
    toast.success(`Approved ${approved} escalation${approved !== 1 ? 's' : ''}`)
  }, [selectedIds, resolve])

  if (!projectId) return null

  if (isLoading) return <QueueSkeleton />

  if (isError) {
    return (
      <div className="dashboard-section flex items-center gap-2 px-1 py-3 text-xs text-[var(--red)]">
        <AlertCircle size={14} />
        <span>Failed to load escalations: {error instanceof Error ? error.message : 'Unknown error'}</span>
      </div>
    )
  }

  if (!escalations || escalations.length === 0) {
    return null
  }

  const sorted = sortEscalations(escalations)
  const criticalCount = sorted.filter(a => a.urgency === 'critical').length

  return (
    <section className={cn('dashboard-section', focusMode && 'max-w-none')}>
      {/* Header or bulk action bar */}
      {selectedIds.size > 0 ? (
        <div className="dashboard-escalation-bulkbar mb-3 animate-fade-in">
          <div className="dashboard-escalation-bulkbar-meta min-w-0">
            <div className="dashboard-escalation-bulkbar-title-row">
              <span className="dashboard-escalation-bulkbar-title">
                {selectedIds.size} selected
              </span>
              {escalations.length > 1 && (
                selectedIds.size === escalations.length ? (
                  <span className="dashboard-escalation-bulkbar-status">All selected</span>
                ) : (
                  <button
                    type="button"
                    onClick={toggleAll}
                    className="dashboard-escalation-bulkbar-link"
                  >
                    Select all
                  </button>
                )
              )}
            </div>
            <span className="dashboard-escalation-bulkbar-copy">
              {selectedIds.size === 1
                ? 'Approve it now or keep reviewing.'
                : 'Approve this selection together or keep reviewing.'}
            </span>
          </div>
          <div className="dashboard-escalation-bulkbar-actions">
            <Button
              size="xs"
              onClick={handleBulkApprove}
              disabled={resolve.isPending}
              className="dashboard-escalation-bulkbar-approve text-xs"
            >
              <CheckCircle2 size={11} className="mr-1" />
              Approve selected
            </Button>
            <Button
              size="xs"
              variant="ghost"
              onClick={() => setSelectedIds(new Set())}
              className="dashboard-escalation-bulkbar-clear text-xs text-[var(--text-3)]"
            >
              Clear
            </Button>
          </div>
        </div>
      ) : (
        <div className="dashboard-section-heading mb-3">
          <div className="dashboard-section-heading-main">
              <div className="flex items-center gap-2">
                <AlertTriangle size={14} className="text-[var(--amber)]" />
                <span className={cn('dashboard-section-title', focusMode ? 'text-base' : 'text-sm')}>
                  Needs You
                </span>
              </div>
              <p className="dashboard-section-caption">One clear decision here gets the work moving again.</p>
            </div>
          <div className="dashboard-section-meta">
            <span className="dashboard-section-counter text-[var(--amber)]">
              {escalations.length} pending
            </span>
            {criticalCount > 0 && (
              <span className="dashboard-section-counter text-[var(--red)]">
                {criticalCount} critical
              </span>
            )}
          </div>
        </div>
      )}
      <div className={cn(
        'dashboard-list-surface dashboard-list-surface-flat dashboard-side-surface',
        focusMode ? 'max-w-none overflow-hidden' : sorted.length > 10 ? 'max-h-[400px] overflow-y-auto overflow-x-hidden' : 'overflow-hidden',
      )}>
        {sorted.map(esc => (
          <div
            key={esc.id}
            className={cn(
              'dashboard-queue-selection flex items-center',
              selectedIds.has(esc.id) && 'dashboard-queue-row-selected',
            )}
          >
            <div className="flex items-center self-center pl-3 shrink-0">
              <Checkbox
                checked={selectedIds.has(esc.id)}
                onCheckedChange={() => toggleSelection(esc.id)}
                aria-label={`Select escalation ${esc.title}`}
                className="dashboard-queue-checkbox mr-0"
              />
            </div>
            <div className="flex-1 min-w-0">
              <EscalationRow
                esc={esc}
                initialExpanded={esc.id === initialSelectedId}
                onResolve={(target, resolution) => setResolveTarget({ esc: target, resolution })}
                onQuickApprove={handleQuickApprove}
                focusMode={focusMode}
              />
            </div>
          </div>
        ))}
      </div>

      <ResolveDialog
        escalation={resolveTarget?.esc ?? null}
        initialResolution={resolveTarget?.resolution ?? null}
        projectId={projectId}
        onClose={() => setResolveTarget(null)}
      />
    </section>
  )
}
