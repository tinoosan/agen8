import { useMemo } from 'react'
import { Link } from 'wouter'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, ListChecks, CircleDashed, CircleDot, Ban, Eye, CircleCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { filteredTasksLink } from '../../lib/routing'
import { taskStatusColor, taskStatusLabel } from '../../lib/statusLabels'
import type { Task } from '../../lib/types'

/* ── Status buckets ───────────────────────────────────────
 * Mirrors the canonical task lifecycle (pending → active →
 * in_review → succeeded/failed; active ↔ blocked). Terminal
 * `failed`/`canceled` are intentionally omitted — the strip
 * surfaces live work plus recently-completed, not the graveyard.
 *
 * Labels and colors come from the shared statusLabels helpers so the
 * tiles speak the same language as the Tasks panel and every other view. */

type Bucket = {
  key: string
  match: (status: string) => boolean
  Icon: LucideIcon
}

const BUCKETS: Bucket[] = [
  { key: 'pending', match: (s) => s === 'pending', Icon: CircleDashed },
  { key: 'active', match: (s) => s === 'active', Icon: CircleDot },
  { key: 'blocked', match: (s) => s === 'blocked', Icon: Ban },
  { key: 'in_review', match: (s) => s === 'in_review', Icon: Eye },
  { key: 'succeeded', match: (s) => s === 'succeeded', Icon: CircleCheck },
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

function StatCell({ projectId, bucket, count }: { projectId: string; bucket: Bucket; count: number }) {
  const hasCount = count > 0
  // Number and icon share the status accent; an empty bucket recedes to grey
  // so a busy column reads first. Colour comes from the shared helper.
  const accent = hasCount ? taskStatusColor(bucket.key) : 'var(--text-3)'
  const Icon = bucket.Icon
  return (
    <Link
      to={filteredTasksLink(projectId, bucket.key)}
      aria-label={`${count} ${taskStatusLabel(bucket.key)} — open filtered Tasks list`}
      className="flex flex-col gap-1.5 rounded-[16px] border border-[var(--border)] bg-[var(--bg-elevated)] px-3 py-4 no-underline transition-colors hover:bg-[var(--bg-hover)]"
    >
      <span
        className="text-[1.75rem] font-semibold leading-none tracking-[-0.02em] tabular-nums"
        style={{ color: accent }}
      >
        {count}
      </span>
      {/* icon decorates the label rather than floating above the digit;
          rem sizing keeps both in step with the user's font scale */}
      <div className="flex items-center gap-1 text-[var(--text-3)]">
        <Icon size="0.75rem" style={{ color: accent }} aria-hidden />
        <span className="text-[0.625rem] font-semibold uppercase tracking-[0.06em]">
          {taskStatusLabel(bucket.key)}
        </span>
      </div>
    </Link>
  )
}

/* ── Main exported component ──────────────────────────── */

export default function TaskSummary({ projectId }: { projectId: string | null }) {
  const { data: tasks, isLoading, isError, error } = useProjectTasks(projectId)

  const counts = useMemo(() => countByBucket(tasks ?? []), [tasks])

  if (!projectId) return null

  if (isLoading) {
    return (
      <div className="grid grid-cols-3 sm:grid-cols-5 gap-2 max-w-[560px]">
        {BUCKETS.map((b) => (
          <Skeleton key={b.key} className="h-[5.25rem] rounded-[var(--r-lg)]" />
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
      <div className="grid grid-cols-3 sm:grid-cols-5 gap-2.5 max-w-[560px]">
        {BUCKETS.map((bucket) => (
          <StatCell key={bucket.key} projectId={projectId} bucket={bucket} count={counts[bucket.key]} />
        ))}
      </div>
    </section>
  )
}
