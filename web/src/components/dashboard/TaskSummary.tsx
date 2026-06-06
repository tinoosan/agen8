import { useMemo } from 'react'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, ListChecks, CircleDashed, CircleDot, Ban, Eye, CircleCheck } from 'lucide-react'
import type { LucideIcon } from 'lucide-react'
import { useProjectTasks, useProjectTasksSSE } from '../../hooks/useProjectTasks'
import type { Task } from '../../lib/types'

/* ── Status buckets ───────────────────────────────────────
 * Mirrors the canonical task lifecycle (pending → active →
 * in_review → succeeded/failed; active ↔ blocked). Terminal
 * `failed`/`canceled` are intentionally omitted — the strip
 * surfaces live work plus recently-completed, not the graveyard.
 *
 * `color` doubles as the tile tint source. `succeeded` uses a muted
 * tone so settled work recedes and live counts dominate. */

type Bucket = {
  key: string
  label: string
  match: (status: string) => boolean
  color: string
  Icon: LucideIcon
}

const BUCKETS: Bucket[] = [
  { key: 'pending', label: 'Queued', match: (s) => s === 'pending', color: 'var(--text-2)', Icon: CircleDashed },
  { key: 'active', label: 'Active', match: (s) => s === 'active', color: 'var(--green)', Icon: CircleDot },
  { key: 'blocked', label: 'Blocked', match: (s) => s === 'blocked', color: 'var(--red)', Icon: Ban },
  { key: 'in_review', label: 'Review', match: (s) => s === 'in_review', color: 'var(--amber)', Icon: Eye },
  { key: 'succeeded', label: 'Done', match: (s) => s === 'succeeded', color: 'var(--text-3)', Icon: CircleCheck },
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
  const hasCount = count > 0
  const accent = hasCount ? bucket.color : 'var(--text-3)'
  // Borderless: the tile is a soft tint of its status colour (calm grey
  // when empty), so colour — not chrome — carries the meaning.
  const tint = hasCount
    ? `color-mix(in srgb, ${bucket.color} 13%, var(--bg-app))`
    : `color-mix(in srgb, var(--text-3) 6%, var(--bg-app))`
  const Icon = bucket.Icon
  return (
    <div
      className="flex flex-col gap-1.5 rounded-[var(--r-lg)] px-3 py-2.5"
      style={{ background: tint }}
    >
      <span
        className="text-[1.5rem] font-semibold leading-none tabular-nums"
        style={{ color: accent }}
      >
        {count}
      </span>
      {/* icon decorates the label rather than floating above the digit;
          rem sizing keeps both in step with the user's font scale */}
      <div className="flex items-center gap-1.5 text-[var(--text-3)]">
        <Icon size="0.875rem" style={{ color: accent }} aria-hidden />
        <span className="text-[0.75rem] font-medium">{bucket.label}</span>
      </div>
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
      <div className="grid grid-cols-3 sm:grid-cols-5 gap-2 max-w-[560px]">
        {BUCKETS.map((bucket) => (
          <StatCell key={bucket.key} bucket={bucket} count={counts[bucket.key]} />
        ))}
      </div>
    </section>
  )
}
