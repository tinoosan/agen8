import { useMemo } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, ListChecks } from 'lucide-react'
import { cn } from '@/lib/utils'
import { useProjectTasks, useProjectTasksSSE } from '../../hooks/useProjectTasks'
import type { Task } from '../../lib/types'

/* ── Status buckets ───────────────────────────────────────
 * Mirrors the canonical task lifecycle (pending → active →
 * in_review → succeeded/failed; active ↔ blocked). Terminal
 * `failed`/`canceled` are intentionally omitted — the strip
 * surfaces live work plus recently-completed, not the graveyard. */

type Bucket = {
  key: string
  label: string
  match: (status: string) => boolean
  color: string
  alert?: boolean
}

const BUCKETS: Bucket[] = [
  { key: 'pending', label: 'Queued', match: (s) => s === 'pending', color: 'var(--text-2)' },
  { key: 'active', label: 'In progress', match: (s) => s === 'active', color: 'var(--green)' },
  { key: 'blocked', label: 'Blocked', match: (s) => s === 'blocked', color: 'var(--red)', alert: true },
  { key: 'in_review', label: 'In review', match: (s) => s === 'in_review', color: 'var(--amber)' },
  { key: 'succeeded', label: 'Done', match: (s) => s === 'succeeded', color: 'var(--text-3)' },
]

function countByBucket(tasks: Task[]): Record<string, number> {
  const counts: Record<string, number> = {}
  for (const bucket of BUCKETS) counts[bucket.key] = 0
  for (const task of tasks) {
    const status = task.status ?? ''
    const bucket = BUCKETS.find((b) => b.match(status))
    if (bucket) counts[bucket.key] += 1
  }
  return counts
}

/* ── Stat cell ────────────────────────────────────────── */

function StatCell({ bucket, count }: { bucket: Bucket; count: number }) {
  const active = bucket.alert && count > 0
  return (
    <div
      className={cn(
        'dashboard-list-surface flex flex-col gap-0.5 px-3 py-2.5',
        active && 'ring-1 ring-[var(--red)]/40',
      )}
    >
      <span
        className="text-[1.375rem] font-semibold leading-none tabular-nums"
        style={{ color: count > 0 ? bucket.color : 'var(--text-3)' }}
      >
        {count}
      </span>
      <span className="text-[0.625rem] font-semibold uppercase tracking-[0.06em] text-[var(--text-3)]">
        {bucket.label}
      </span>
    </div>
  )
}

/* ── Main exported component ──────────────────────────── */

export default function TaskSummary({ projectId }: { projectId: string | null }) {
  useProjectTasksSSE()
  const { data: tasks, isLoading, isError, error } = useProjectTasks(projectId)

  const counts = useMemo(() => countByBucket(tasks ?? []), [tasks])

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="grid grid-cols-3 sm:grid-cols-5 gap-2 max-w-[560px]">
        {BUCKETS.map((b) => (
          <Skeleton key={b.key} className="h-[58px] rounded-[var(--r-lg)]" />
        ))}
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center gap-2 px-1 py-3 text-[0.6875rem] text-[var(--red)]">
        <AlertCircle size={13} />
        <span>Failed to load tasks: {error instanceof Error ? error.message : 'Unknown error'}</span>
      </div>
    )
  }

  // Nothing to summarize until the project has at least one task.
  if (!tasks || tasks.length === 0) return null

  return (
    <section className="dashboard-section">
      <div className="dashboard-section-heading mb-2">
        <div className="dashboard-section-heading-main">
          <div className="flex items-center gap-2">
            <ListChecks size={14} className="text-[var(--accent)]" />
            <span className="dashboard-section-title">Tasks</span>
          </div>
          <p className="dashboard-section-caption">The work in flight, grouped by where it stands.</p>
        </div>
        <div className="dashboard-section-meta">
          <span className="dashboard-section-counter">{tasks.length} total</span>
        </div>
      </div>
      <div className="grid grid-cols-3 sm:grid-cols-5 gap-2 max-w-[560px]">
        {BUCKETS.map((bucket) => (
          <StatCell key={bucket.key} bucket={bucket} count={counts[bucket.key]} />
        ))}
      </div>
    </section>
  )
}
