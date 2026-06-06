import { useState } from 'react'
import { ArrowLeft, ScrollText, Download, ChevronRight, Clock, Link2, Trash2, Search, ArrowDownUp } from 'lucide-react'
import { decisionsPanelLink, useNavigation } from '../lib/routing'
import { useDecisionLog, useDecisionStats, useExportDecisions, useDeleteDecision } from '../hooks/useDecisions'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DatePicker } from '@/components/ui/date-picker'
import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
  AlertDialogTrigger,
} from '@/components/ui/alert-dialog'
import type { DecisionView } from '../lib/types'
import DecisionDetails from '../components/decision/DecisionDetails'
import { toast } from 'sonner'
import { Link } from 'wouter'

const PAGE_SIZE = 20

function timeAgo(iso: string): string {
  const diffMs = Date.now() - new Date(iso).getTime()
  if (diffMs < 0) return 'just now'
  const minutes = Math.floor(diffMs / 60_000)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

// Confidence is the headline signal of a decision, so colour-code it for scanning:
// high (>=80%) green, medium (>=50%) amber, low red. Literal class strings keep
// Tailwind's JIT happy (no dynamic interpolation).
function confidenceClass(confidence: number): string {
  if (confidence >= 0.8) return 'bg-[var(--green-dim)] text-[var(--green)]'
  if (confidence >= 0.5) return 'bg-[var(--amber-dim)] text-[var(--amber)]'
  return 'bg-[var(--red-dim)] text-[var(--red)]'
}

function downloadBlob(blob: Blob, filename: string) {
  const url = URL.createObjectURL(blob)
  const link = document.createElement('a')
  link.href = url
  link.download = filename
  document.body.appendChild(link)
  link.click()
  link.remove()
  URL.revokeObjectURL(url)
}

function exportCsv(decisions: DecisionView[]) {
  const header = 'id,title,source,confidence,createdAt,taskRef,keyResultRef,missionRef,rationale,alternativesRejected\n'
  const rows = decisions.map(decision => [
    decision.id,
    decision.title,
    decision.source,
    String(decision.confidence ?? ''),
    decision.createdAt,
    decision.taskRef ?? '',
    decision.keyResultRef ?? '',
    decision.missionRef ?? '',
    decision.rationale,
    decision.alternativesRejected ?? '',
  ].map(value => `"${String(value).replace(/"/g, '""')}"`).join(',')).join('\n')
  downloadBlob(new Blob([header + rows], { type: 'text/csv;charset=utf-8' }), 'decisions.csv')
}

function DecisionRow({ decision }: { decision: DecisionView }) {
  const [expanded, setExpanded] = useState(false)
  const deleteDecision = useDeleteDecision()

  // Full "Type: id" strings for the expanded panel.
  const refs = [
    decision.missionRef ? `Mission: ${decision.missionRef}` : null,
    decision.taskRef ? `Task: ${decision.taskRef}` : null,
    decision.keyResultRef ? `KR: ${decision.keyResultRef}` : null,
  ].filter(Boolean) as string[]

  // Type-only labels for the collapsed meta — raw refs are opaque ids, so the
  // entity type is the useful at-a-glance signal.
  const refTypes = [
    decision.missionRef ? 'Mission' : null,
    decision.taskRef ? 'Task' : null,
    decision.keyResultRef ? 'KR' : null,
  ].filter(Boolean) as string[]

  const confidencePct = Math.round((decision.confidence ?? 0) * 100)

  const handleDelete = async () => {
    try {
      await deleteDecision.mutateAsync(decision.id)
      toast.success('Decision deleted')
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to delete decision')
    }
  }

  return (
    <div className="group">
      <div className="flex items-start gap-2 px-4 py-3 transition-colors hover:bg-[var(--bg-hover)] sm:px-6">
        <button
          type="button"
          aria-label={`Toggle details for decision ${decision.title}`}
          className="flex min-w-0 flex-1 items-start gap-2.5 border-0 bg-transparent p-0 text-left cursor-pointer"
          onClick={() => setExpanded(prev => !prev)}
        >
          <ChevronRight
            size={14}
            className={`mt-[3px] shrink-0 text-[var(--text-3)] transition-transform ${expanded ? 'rotate-90' : ''}`}
          />
          <div className="min-w-0 flex-1">
            <div className="text-sm font-medium leading-snug text-[var(--text-1)]">{decision.title}</div>
            <div className="mt-1.5 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-[0.6875rem] text-[var(--text-3)]">
              <span
                title={`${confidencePct}% confidence`}
                className={`inline-flex items-center rounded-full px-1.5 py-px text-[0.625rem] font-semibold ${confidenceClass(decision.confidence ?? 0)}`}
              >
                {confidencePct}%
              </span>
              <span className="inline-flex items-center gap-1"><Clock size={10} />{timeAgo(decision.createdAt)}</span>
              {refTypes.map(type => (
                <span
                  key={type}
                  className="inline-flex items-center gap-1 rounded-full bg-[var(--bg-elevated)] px-1.5 py-px text-[0.625rem] text-[var(--text-2)]"
                >
                  <Link2 size={9} className="opacity-70" />{type}
                </span>
              ))}
              {decision.memberName && <span className="truncate">by {decision.memberName}</span>}
            </div>
          </div>
        </button>
        <AlertDialog>
          <AlertDialogTrigger asChild>
            <Button
              type="button"
              variant="ghost-danger"
              size="icon"
              className="h-7 w-7 shrink-0 opacity-100 transition-opacity md:opacity-0 md:group-hover:opacity-100 md:group-focus-within:opacity-100"
              aria-label={`Delete decision ${decision.title}`}
            >
              <Trash2 size={13} />
            </Button>
          </AlertDialogTrigger>
          <AlertDialogContent>
            <AlertDialogHeader>
              <AlertDialogTitle>Delete decision?</AlertDialogTitle>
              <AlertDialogDescription>
                This removes the decision from the log and clears its graph links. This cannot be undone.
              </AlertDialogDescription>
            </AlertDialogHeader>
            <AlertDialogFooter>
              <AlertDialogCancel>Cancel</AlertDialogCancel>
              <AlertDialogAction
                className="bg-[var(--red)] text-white hover:bg-[var(--red)]/90"
                onClick={() => void handleDelete()}
              >
                Delete decision
              </AlertDialogAction>
            </AlertDialogFooter>
          </AlertDialogContent>
        </AlertDialog>
      </div>

      {expanded && (
        <div className="px-4 pb-4 sm:px-6">
          <div className="pl-[26px]">
            <DecisionDetails decision={decision} />
            {refs.length > 0 && (
              <>
                <div className="mt-3 mb-1.5 text-[0.625rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]">Linked entities</div>
                <div className="flex flex-wrap gap-1.5">
                  {refs.map(ref => (
                    <span
                      key={ref}
                      className="inline-flex items-center gap-1 rounded-[var(--r-sm)] bg-[var(--accent-dim)] px-2 py-0.5 text-[0.6875rem] text-[var(--accent)]"
                    >
                      <Link2 size={10} />
                      {ref}
                    </span>
                  ))}
                </div>
              </>
            )}
          </div>
        </div>
      )}
    </div>
  )
}

// StatCard is a compact filled tile — no border, matching the date-picker
// popover treatment. The value turns red/amber only when the count is non-zero,
// so the eye lands on the decisions that actually need attention.
function StatCard({ label, value, tone, hint }: {
  label: string
  value: number
  tone?: 'danger' | 'warning'
  hint?: string
}) {
  const valueColor =
    value > 0 && tone === 'danger'
      ? 'text-[var(--red)]'
      : value > 0 && tone === 'warning'
        ? 'text-[var(--amber)]'
        : 'text-[var(--text-1)]'
  return (
    <div title={hint} className="rounded-[var(--r-md)] bg-[var(--bg-elevated)] px-3 py-2.5">
      <div className="text-[0.625rem] font-medium uppercase tracking-[0.04em] text-[var(--text-3)]">{label}</div>
      <div className={`mt-1 text-xl font-semibold tabular-nums ${valueColor}`}>{value}</div>
    </div>
  )
}

export default function Decisions() {
  const { projectId } = useNavigation()
  const [query, setQuery] = useState('')
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')
  const [sort, setSort] = useState<'newest' | 'oldest'>('newest')
  const [page, setPage] = useState(1)
  const exportDecisions = useExportDecisions()

  const logQuery = useDecisionLog(projectId, {
    page,
    pageSize: PAGE_SIZE,
    query,
    since: fromDate ? new Date(`${fromDate}T00:00:00Z`).toISOString() : undefined,
    until: toDate ? new Date(`${toDate}T23:59:59Z`).toISOString() : undefined,
    sort,
  })

  // Stats span the whole filtered set (server-side aggregate), so they share the
  // content filters but not sort/page. Called before the projectId guard below
  // to keep hook order stable.
  const statsQuery = useDecisionStats(projectId, {
    query,
    since: fromDate ? new Date(`${fromDate}T00:00:00Z`).toISOString() : undefined,
    until: toDate ? new Date(`${toDate}T23:59:59Z`).toISOString() : undefined,
  })

  const total = logQuery.data?.total ?? 0
  const decisions = logQuery.data?.decisions ?? []
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))

  if (!projectId) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-8">
        <ScrollText size={40} className="text-[var(--text-3)] opacity-40 mb-4" />
        <h2 className="text-lg font-semibold text-[var(--text-1)] mb-1">No project selected</h2>
        <p className="text-sm text-[var(--text-3)]">Select a project from the sidebar to view the decision log.</p>
      </div>
    )
  }

  const onExportCsv = async () => {
    const exported = await exportDecisions.mutateAsync({
      projectId,
      query: query || undefined,
      since: fromDate ? new Date(`${fromDate}T00:00:00Z`).toISOString() : undefined,
      until: toDate ? new Date(`${toDate}T23:59:59Z`).toISOString() : undefined,
      sort,
    })
    exportCsv(exported)
  }

  return (
    <div className="flex flex-col h-full">
      {/* Header */}
      <header className="shrink-0 border-b border-[var(--border)]">
        <div className="mx-auto flex w-full max-w-4xl flex-wrap items-center gap-3 px-4 py-3 sm:px-6">
          <Button asChild variant="ghost" size="sm" className="gap-1.5">
            <Link to={decisionsPanelLink(projectId)}>
              <ArrowLeft size={14} />
              Dashboard
            </Link>
          </Button>
          <div className="hidden md:flex items-center gap-2.5">
            <ScrollText size={18} className="text-[var(--accent)] shrink-0" />
            <h1 className="m-0 text-lg font-semibold tracking-[-0.02em] text-[var(--text-1)]">Decision Log</h1>
            {total > 0 && <Badge variant="secondary" className="text-xs">{total}</Badge>}
          </div>
          <div className="ml-auto flex items-center gap-1">
            <Button
              variant="ghost"
              size="sm"
              className="gap-1.5"
              onClick={() => {
                setSort(prev => (prev === 'newest' ? 'oldest' : 'newest'))
                setPage(1)
              }}
            >
              <ArrowDownUp size={13} />
              {sort === 'newest' ? 'Newest first' : 'Oldest first'}
            </Button>
            <Button variant="ghost" size="sm" className="gap-1.5" onClick={onExportCsv} disabled={exportDecisions.isPending}>
              <Download size={13} /> CSV
            </Button>
          </div>
        </div>
      </header>

      {/* Filter bar */}
      <div className="shrink-0 border-b border-[var(--border)]">
        <div className="mx-auto flex w-full max-w-4xl flex-wrap items-center gap-2 px-4 py-2.5 sm:px-6">
          <div className="relative min-w-[180px] flex-1">
            <Search size={14} className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)]" />
            <Input
              id="decision-search"
              className="h-9 pl-8"
              placeholder="Search decisions..."
              value={query}
              onChange={(event) => {
                setQuery(event.target.value)
                setPage(1)
              }}
            />
          </div>
          <div className="flex w-full items-center gap-1.5 sm:w-auto">
            <Label htmlFor="decision-from-date" className="text-[0.6875rem] text-[var(--text-3)]">From</Label>
            <DatePicker
              id="decision-from-date"
              className="min-w-0 flex-1 sm:w-[150px] sm:flex-none"
              placeholder="Start date"
              value={fromDate}
              max={toDate || undefined}
              onChange={(value) => {
                setFromDate(value)
                setPage(1)
              }}
            />
            <Label htmlFor="decision-to-date" className="text-[0.6875rem] text-[var(--text-3)]">To</Label>
            <DatePicker
              id="decision-to-date"
              className="min-w-0 flex-1 sm:w-[150px] sm:flex-none"
              placeholder="End date"
              value={toDate}
              min={fromDate || undefined}
              onChange={(value) => {
                setToDate(value)
                setPage(1)
              }}
            />
          </div>
        </div>
      </div>

      {/* Summary */}
      {statsQuery.data && statsQuery.data.total > 0 && (
        <div className="shrink-0 border-b border-[var(--border)]">
          <div className="mx-auto grid w-full max-w-4xl grid-cols-2 gap-2 px-4 py-3 sm:grid-cols-4 sm:px-6">
            <StatCard label="Total" value={statsQuery.data.total} hint="Decisions matching the current filters" />
            <StatCard label="Needs review" value={statsQuery.data.lowConfidence} tone="danger" hint="Logged with confidence below 50%" />
            <StatCard label="Unlinked" value={statsQuery.data.unlinked} tone="warning" hint="Not linked to a task, key result, or mission" />
            <StatCard label="Revisit conditions" value={statsQuery.data.withInvalidationConditions} hint="Recorded conditions that would invalidate the decision" />
          </div>
        </div>
      )}

      {/* Content */}
      <div className="flex-1 overflow-y-auto">
        {logQuery.isLoading ? (
          <div className="mx-auto w-full max-w-4xl px-4 py-8 text-sm text-[var(--text-3)] sm:px-6">Loading decisions...</div>
        ) : decisions.length === 0 ? (
          <div className="mx-auto flex h-full max-w-4xl flex-col items-center justify-center gap-2 px-6 text-center">
            <ScrollText size={36} className="text-[var(--text-3)] opacity-30" />
            <h3 className="m-0 text-base font-semibold text-[var(--text-1)]">No decisions found</h3>
            <p className="m-0 max-w-[340px] text-sm leading-relaxed text-[var(--text-3)]">
              {query || fromDate || toDate
                ? 'Try adjusting your search or date range.'
                : 'Decisions are logged here as agents make choices during their work.'}
            </p>
          </div>
        ) : (
          <div className="mx-auto w-full max-w-4xl divide-y divide-[var(--border)]">
            {decisions.map(decision => <DecisionRow key={decision.id} decision={decision} />)}
          </div>
        )}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        <div className="shrink-0 border-t border-[var(--border)]">
          <div className="mx-auto flex w-full max-w-4xl items-center justify-between px-4 py-2.5 text-sm text-[var(--text-3)] sm:px-6">
            <span>Page {page} of {totalPages}</span>
            <div className="flex items-center gap-2">
              <Button variant="ghost" size="sm" onClick={() => setPage(prev => Math.max(1, prev - 1))} disabled={page <= 1}>
                Previous
              </Button>
              <Button variant="ghost" size="sm" onClick={() => setPage(prev => Math.min(totalPages, prev + 1))} disabled={page >= totalPages}>
                Next
              </Button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
