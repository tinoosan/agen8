import { useState } from 'react'
import { Link } from 'wouter'
import {
  ScrollText,
  Download,
  Clock,
  Link2,
  Unlink,
  AlertTriangle,
  Search,
  ArrowDownUp,
  ChevronRight,
} from 'lucide-react'
import { useNavigation, decisionDetailLink } from '../lib/routing'
import ListPager from '../components/ListPager'
import StatTile from '../components/StatTile'
import { usePageParam } from '../hooks/usePageParam'
import { useDecisionLog, useDecisionStats, useExportDecisions } from '../hooks/useDecisions'
import { sanitizeDecisionTitle } from '../lib/displaySanitizers'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Label } from '@/components/ui/label'
import { DatePicker } from '@/components/ui/date-picker'
import { Skeleton } from '@/components/ui/skeleton'
import RelativeTime from '@/components/RelativeTime'
import { confidenceBadgeClass } from '@/lib/decisionDisplay'
import { cn } from '@/lib/utils'
import type { DecisionView } from '../lib/types'

/*
 * Decisions — the project's decision log on its own page (formerly a wrapper
 * around the rail-era DashboardDecisionsPanel). Keeps the full log machinery
 * (search, date range, sort, CSV export, ref + confidence chips, pagination)
 * and adds aggregate tiles from the server-side decision.stats so landing here
 * answers "how many decisions, how many are shaky or untraceable" at a glance.
 * Tiles reflect the active filters, mirroring the rows below them.
 */

const PAGE_SIZE = 20

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
  const header =
    'id,title,source,confidence,createdAt,taskRef,keyResultRef,missionRef,rationale,alternativesRejected\n'
  const rows = decisions
    .map((decision) =>
      [
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
      ]
        .map((value) => `"${String(value).replace(/"/g, '""')}"`)
        .join(','),
    )
    .join('\n')
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
      className="group flex items-start gap-2.5 px-4 py-2.5 no-underline transition-colors hover:bg-[var(--bg-hover)]"
    >
      <div className="min-w-0 flex-1">
        <div className="text-[0.875rem] font-medium leading-snug text-[var(--text-1)] transition-colors group-hover:text-[var(--accent)]">
          {sanitizeDecisionTitle(decision.title)}
        </div>
        <div className="mt-1 flex flex-wrap items-center gap-x-2.5 gap-y-1 text-[0.6875rem] text-[var(--text-3)]">
          <span
            title={`${confidencePct}% confidence`}
            className={`inline-flex items-center rounded-full px-1.5 py-px text-[0.625rem] font-semibold ${confidenceBadgeClass(decision.confidence ?? 0)}`}
          >
            {confidencePct}%
          </span>
          <span className="inline-flex items-center gap-1">
            <Clock size={10} />
            <RelativeTime iso={decision.createdAt} />
          </span>
          {refTypes.map((type) => (
            <span
              key={type}
              className="inline-flex items-center gap-1 rounded-full bg-[var(--bg-surface,var(--bg-hover))] px-1.5 py-px text-[0.625rem] text-[var(--text-2)]"
            >
              <Link2 size={9} className="opacity-70" />
              {type}
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

/* ── Page ─────────────────────────────────────────────── */

export default function Decisions() {
  const { projectId } = useNavigation()
  const [query, setQuery] = useState('')
  const [fromDate, setFromDate] = useState('')
  const [toDate, setToDate] = useState('')
  const [sort, setSort] = useState<'newest' | 'oldest'>('newest')
  const [page, setPage] = usePageParam()
  const exportDecisions = useExportDecisions()

  const since = isoStart(fromDate)
  const until = isoEnd(toDate)

  const logQuery = useDecisionLog(projectId, { page, pageSize: PAGE_SIZE, query, since, until, sort })
  // Tiles summarize the SAME filtered set as the rows (stats span all matching
  // rows, not just the current page) — server-computed, so they're accurate.
  const statsQuery = useDecisionStats(projectId, { query, since, until })

  const total = logQuery.data?.total ?? 0
  const decisions = logQuery.data?.decisions ?? []
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE))
  const hasActiveFilters = !!(query || fromDate || toDate)
  const stats = statsQuery.data

  if (!projectId) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
          Select a project to view the decision log.
        </div>
      </div>
    )
  }

  const onExportCsv = async () => {
    const exported = await exportDecisions.mutateAsync({ projectId, query: query || undefined, since, until, sort })
    exportCsv(exported)
  }

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-[960px] px-6 pt-8 pb-12">
        {/* Header */}
        <div className="mb-5 flex items-center justify-between gap-3">
          <h1 className="m-0 flex items-center gap-2 text-[1.75rem] font-bold leading-[1.14] tracking-[-0.03em] text-[var(--text-1)]">
            <ScrollText size={22} className="text-[var(--accent)]" aria-hidden />
            Decision Log
          </h1>
          <div className="flex items-center gap-0.5">
            <Button
              variant="ghost"
              size="sm"
              className="gap-1.5"
              onClick={() => {
                setSort((prev) => (prev === 'newest' ? 'oldest' : 'newest'))
                setPage(1)
              }}
            >
              <ArrowDownUp size={13} />
              {sort === 'newest' ? 'Newest first' : 'Oldest first'}
            </Button>
            <Button
              variant="ghost"
              size="sm"
              className="gap-1.5"
              onClick={onExportCsv}
              disabled={exportDecisions.isPending}
            >
              <Download size={13} /> CSV
            </Button>
          </div>
        </div>

        {/* Aggregate tiles — reflect the active filters */}
        {stats && (
          <div className="@container mb-6">
            <div className="grid grid-cols-3 gap-3">
              <StatTile label="Total" value={stats.total} tone="var(--accent)" icon={ScrollText} />
              <StatTile
                label="Low confidence"
                value={stats.lowConfidence}
                sub="worth revisiting"
                tone={stats.lowConfidence > 0 ? 'var(--amber)' : 'var(--text-3)'}
                icon={AlertTriangle}
              />
              <StatTile
                label="Unlinked"
                value={stats.unlinked}
                sub="no mission/task"
                tone={stats.unlinked > 0 ? 'var(--amber)' : 'var(--text-3)'}
                icon={Unlink}
              />
            </div>
          </div>
        )}

        {/* Filters */}
        <div className="mb-4 flex flex-wrap items-center gap-2">
          <div className="relative min-w-[180px] flex-1">
            <Search
              size={14}
              className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)]"
            />
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
            <Label htmlFor="decision-from-date" className="text-[0.6875rem] text-[var(--text-3)]">
              From
            </Label>
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
            <Label htmlFor="decision-to-date" className="text-[0.6875rem] text-[var(--text-3)]">
              To
            </Label>
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

        {/* Rows */}
        <div className="overflow-hidden rounded-[14px] border border-[var(--border)] bg-[var(--bg-elevated)]">
          {logQuery.isLoading ? (
            <div className="flex flex-col">
              {[1, 2, 3, 4].map((i) => (
                <div key={i} className="flex flex-col gap-2 px-4 py-2.5">
                  <Skeleton className="h-3.5 w-3/4 max-w-[260px]" />
                  <div className="flex items-center gap-2">
                    <Skeleton className="h-3 w-10 rounded-full" />
                    <Skeleton className="h-3 w-14" />
                  </div>
                </div>
              ))}
            </div>
          ) : decisions.length === 0 ? (
            <div className="flex flex-col items-center justify-center py-16 text-center">
              <ScrollText size={36} className="mb-4 text-[var(--text-3)] opacity-25" />
              <h3 className="mb-1.5 text-[1.0625rem] font-semibold tracking-[-0.01em] text-[var(--text-1)]">
                {hasActiveFilters ? 'No results' : 'No decisions yet'}
              </h3>
              <p className="mb-5 max-w-sm text-[0.875rem] text-[var(--text-3)]">
                {hasActiveFilters
                  ? 'Try adjusting your search or date range.'
                  : 'Decisions are logged here as agents make choices during their work.'}
              </p>
              {hasActiveFilters && (
                <Button
                  variant="secondary"
                  onClick={() => {
                    setQuery('')
                    setFromDate('')
                    setToDate('')
                    setPage(1)
                  }}
                >
                  Clear filters
                </Button>
              )}
            </div>
          ) : (
            decisions.map((decision, i) => (
              <div key={decision.id} className={cn(i > 0 && 'border-t border-[var(--border)]')}>
                <DecisionLogRow projectId={projectId} decision={decision} />
              </div>
            ))
          )}
        </div>

        {/* Pagination */}
        {totalPages > 1 && (
          <div className="mt-4">
            <ListPager page={page} totalPages={totalPages} onPageChange={setPage} />
          </div>
        )}
      </div>
    </div>
  )
}
