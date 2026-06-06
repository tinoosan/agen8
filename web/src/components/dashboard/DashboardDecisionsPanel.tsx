import { useState } from 'react'
import { Link } from 'wouter'
import {
  ArrowLeft,
  ScrollText,
  Download,
  Clock,
  Link2,
  Search,
  ArrowDownUp,
  ChevronRight,
} from 'lucide-react'
import { decisionsPanelLink, decisionDetailLink } from '../../lib/routing'
import { useDecisionLog, useDecisionStats, useExportDecisions } from '../../hooks/useDecisions'
import { sanitizeDecisionTitle } from '../../lib/displaySanitizers'
import { Badge } from '@/components/ui/badge'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DatePicker } from '@/components/ui/date-picker'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import type { DecisionView } from '../../lib/types'

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

// Confidence is the headline signal, so colour-code it for scanning: high
// (>=80%) green, medium (>=50%) amber, low red. Literal class strings keep
// Tailwind's JIT happy (no dynamic interpolation).
function confidenceClass(confidence: number): string {
  if (confidence >= 0.8) return 'bg-[var(--green-dim)] text-[var(--green)]'
  if (confidence >= 0.5) return 'bg-[var(--amber-dim)] text-[var(--amber)]'
  return 'bg-[var(--red-dim)] text-[var(--red)]'
}

function isoStart(date: string): string | undefined {
  return date ? new Date(`${date}T00:00:00Z`).toISOString() : undefined
}

function isoEnd(date: string): string | undefined {
  return date ? new Date(`${date}T23:59:59Z`).toISOString() : undefined
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

/* ── Single decision row — a link to the routed detail page ── */

function DecisionLogRow({ projectId, decision }: { projectId: string; decision: DecisionView }) {
  const refTypes = [
    decision.missionRef ? 'Mission' : null,
    decision.taskRef ? 'Task' : null,
    decision.keyResultRef ? 'KR' : null,
  ].filter(Boolean) as string[]
  const confidencePct = Math.round((decision.confidence ?? 0) * 100)

  return (
    <Link
      to={decisionDetailLink(projectId, decision.id)}
      className="group flex items-start gap-2.5 rounded-[var(--r-md)] px-3 py-2.5 no-underline transition-colors hover:bg-[var(--bg-hover)]"
    >
      <div className="min-w-0 flex-1">
        <div className="text-[0.8125rem] font-medium leading-snug text-[var(--text-1)] transition-colors group-hover:text-[var(--accent)]">
          {sanitizeDecisionTitle(decision.title)}
        </div>
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
      <ChevronRight
        size={14}
        className="mt-[3px] shrink-0 text-[var(--text-3)] opacity-40 transition-opacity group-hover:opacity-70"
      />
    </Link>
  )
}

/* ── Summary metric tile ── */

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

/* ── Loading skeleton ── */

function DecisionsSkeleton({ embedded }: { embedded: boolean }) {
  return (
    <div className={cn('flex flex-col gap-0.5', embedded && 'px-1')}>
      {[1, 2, 3, 4].map((i) => (
        <div key={i} className="flex flex-col gap-2 px-3 py-2.5">
          <Skeleton className="h-3.5 w-3/4 max-w-[260px]" />
          <div className="flex items-center gap-2">
            <Skeleton className="h-3 w-10 rounded-full" />
            <Skeleton className="h-3 w-14" />
            <Skeleton className="h-3 w-12 rounded-full" />
          </div>
        </div>
      ))}
    </div>
  )
}

interface DashboardDecisionsPanelProps {
  projectId: string | null
  focusedProjectRoot?: string | null
  embedded?: boolean
}

export default function DashboardDecisionsPanel({
  projectId,
  embedded = false,
}: DashboardDecisionsPanelProps) {
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
    since: isoStart(fromDate),
    until: isoEnd(toDate),
    sort,
  })

  // Stats span the whole filtered set (server-side aggregate), so they share
  // the content filters but not sort/page. Declared before the projectId guard
  // to keep hook order stable.
  const statsQuery = useDecisionStats(projectId, {
    query,
    since: isoStart(fromDate),
    until: isoEnd(toDate),
  })

  const total = logQuery.data?.total ?? 0
  const decisions = logQuery.data?.decisions ?? []
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const hasActiveFilters = !!(query || fromDate || toDate)

  if (!projectId) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-8">
        <ScrollText size={36} className="text-[var(--text-3)] opacity-30 mb-4" />
        <h2 className="text-base font-semibold text-[var(--text-1)] mb-1">No project selected</h2>
        <p className="text-sm text-[var(--text-3)]">Select a project to view the decision log.</p>
      </div>
    )
  }

  const onExportCsv = async () => {
    const exported = await exportDecisions.mutateAsync({
      projectId,
      query: query || undefined,
      since: isoStart(fromDate),
      until: isoEnd(toDate),
      sort,
    })
    exportCsv(exported)
  }

  const sortButton = (
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
  )

  const csvButton = (
    <Button variant="ghost" size="sm" className="gap-1.5" onClick={onExportCsv} disabled={exportDecisions.isPending}>
      <Download size={13} /> CSV
    </Button>
  )

  const filterControls = (
    <>
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
    </>
  )

  const showStats = !!statsQuery.data && statsQuery.data.total > 0
  const statCards = statsQuery.data && (
    <div className={cn('grid gap-2', embedded ? 'grid-cols-2' : 'grid-cols-2 sm:grid-cols-4')}>
      <StatCard label="Total" value={statsQuery.data.total} hint="Decisions matching the current filters" />
      <StatCard label="Needs review" value={statsQuery.data.lowConfidence} tone="danger" hint="Logged with confidence below 50%" />
      <StatCard label="Unlinked" value={statsQuery.data.unlinked} tone="warning" hint="Not linked to a task, key result, or mission" />
      <StatCard label="Revisit conditions" value={statsQuery.data.withInvalidationConditions} hint="Recorded conditions that would invalidate the decision" />
    </div>
  )

  const body = logQuery.isLoading ? (
    <DecisionsSkeleton embedded={embedded} />
  ) : decisions.length === 0 ? (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <ScrollText size={36} className="text-[var(--text-3)] opacity-25 mb-4" />
      <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '1.0625rem', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
        {hasActiveFilters ? 'No results' : 'No decisions yet'}
      </h3>
      <p className="text-[var(--text-3)] mb-5 max-w-sm" style={{ fontSize: '0.875rem', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
        {hasActiveFilters
          ? 'Try adjusting your search or date range.'
          : 'Decisions are logged here as agents make choices during their work.'}
      </p>
      {hasActiveFilters && (
        <Button
          variant="secondary"
          onClick={() => { setQuery(''); setFromDate(''); setToDate(''); setPage(1) }}
          style={{ letterSpacing: '-0.12px' }}
        >
          Clear filters
        </Button>
      )}
    </div>
  ) : (
    <div className="flex flex-col gap-0.5">
      {decisions.map(decision => (
        <DecisionLogRow key={decision.id} projectId={projectId} decision={decision} />
      ))}
    </div>
  )

  const paginationControls = (
    <>
      <span className="tabular-nums">Page {page} of {totalPages}</span>
      <div className="flex items-center gap-2">
        <Button variant="ghost" size="sm" onClick={() => setPage(prev => Math.max(1, prev - 1))} disabled={page <= 1}>
          Previous
        </Button>
        <Button variant="ghost" size="sm" onClick={() => setPage(prev => Math.min(totalPages, prev + 1))} disabled={page >= totalPages}>
          Next
        </Button>
      </div>
    </>
  )

  return (
    <div className="flex h-full min-h-0 flex-col">
      {/* Header */}
      {embedded ? (
        <div className="shrink-0 px-[var(--dashboard-context-gutter)] pt-5 pb-3 border-b border-[color-mix(in_srgb,var(--border)_56%,transparent)]">
          <div className="flex items-start justify-between gap-3">
            <div className="min-w-0">
              <h1
                className="m-0 text-[var(--text-1)]"
                style={{ fontSize: '1.1875rem', fontWeight: 700, letterSpacing: '-0.36px', lineHeight: 1.18 }}
              >
                Decisions
              </h1>
              <p className="m-0 mt-1 text-[var(--text-3)]" style={{ fontSize: '0.75rem', letterSpacing: '-0.12px', lineHeight: 1.45 }}>
                What was decided, why, and what would change our minds.
              </p>
            </div>
            <div className="flex items-center gap-0.5 shrink-0">
              {sortButton}
              {csvButton}
            </div>
          </div>
        </div>
      ) : (
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
              {sortButton}
              {csvButton}
            </div>
          </div>
        </header>
      )}

      {/* Filter bar */}
      {embedded ? (
        <div className="shrink-0 px-[var(--dashboard-context-gutter)] py-3 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)]">
          <div className="flex flex-wrap items-center gap-2">{filterControls}</div>
        </div>
      ) : (
        <div className="shrink-0 border-b border-[var(--border)]">
          <div className="mx-auto flex w-full max-w-4xl flex-wrap items-center gap-2 px-4 py-2.5 sm:px-6">{filterControls}</div>
        </div>
      )}

      {/* Summary */}
      {showStats && (
        embedded ? (
          <div className="shrink-0 px-[var(--dashboard-context-gutter)] py-3 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)]">
            {statCards}
          </div>
        ) : (
          <div className="shrink-0 border-b border-[var(--border)]">
            <div className="mx-auto w-full max-w-4xl px-4 py-3 sm:px-6">{statCards}</div>
          </div>
        )
      )}

      {/* Content */}
      <div className={cn('flex-1 min-h-0 overflow-y-auto', embedded ? 'px-[var(--dashboard-context-gutter)] py-4' : 'px-3 py-3 sm:px-4 max-w-4xl mx-auto w-full')}>
        {body}
      </div>

      {/* Pagination */}
      {totalPages > 1 && (
        embedded ? (
          <div className="shrink-0 border-t border-[color-mix(in_srgb,var(--border)_42%,transparent)] px-[var(--dashboard-context-gutter)] py-2.5 flex items-center justify-between text-[0.75rem] text-[var(--text-3)]">
            {paginationControls}
          </div>
        ) : (
          <div className="shrink-0 border-t border-[var(--border)]">
            <div className="mx-auto flex w-full max-w-4xl items-center justify-between px-4 py-2.5 text-sm text-[var(--text-3)] sm:px-6">
              {paginationControls}
            </div>
          </div>
        )
      )}
    </div>
  )
}
