import { useMemo } from 'react'
import { BarChart3, AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { formatCoarseDuration } from '../../lib/taskTiming'
import { computeMetrics } from '../../lib/metrics'

/* The metrics page reads the task query the dashboard already uses and derives
 * everything client-side via lib/metrics.ts — no metrics RPC or table. It rides
 * the existing SSE invalidation, so the numbers refresh as work moves. See
 * lib/metrics.ts for the projection and its honest-MVP rules.
 */

/* ── Throughput tiles ───────────────────────────────────────────────────── */

type Tone = 'default' | 'amber' | 'green'

function StatTile({
  label,
  value,
  sub,
  tone = 'default',
}: {
  label: string
  value: string
  sub: string
  tone?: Tone
}) {
  const valueColor =
    tone === 'amber'
      ? 'text-[var(--amber)]'
      : tone === 'green'
        ? 'text-[var(--green)]'
        : 'text-[var(--text-1)]'
  return (
    <div className="rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-elevated)] px-4 py-3.5">
      <div className="text-[0.625rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]">
        {label}
      </div>
      <div className={cn('mt-1.5 text-[1.55rem] font-bold leading-none tabular-nums tracking-[-0.02em]', valueColor)}>
        {value}
      </div>
      <div className="mt-1 text-[0.6875rem] text-[var(--text-3)]">{sub}</div>
    </div>
  )
}

/* ── Skeleton / states ──────────────────────────────────────────────────── */

function MetricsSkeleton() {
  return (
    <div className="flex flex-col gap-7">
      <div className="grid grid-cols-2 gap-2.5 @min-[680px]:grid-cols-4">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-[88px] rounded-[var(--r-md)]" />
        ))}
      </div>
    </div>
  )
}

/* ── Main exported component ─────────────────────────────────────────────── */

export default function MetricsPanel({ projectId }: { projectId: string | null }) {
  const tasksQuery = useProjectTasks(projectId)

  const metrics = useMemo(
    () => computeMetrics({ tasks: tasksQuery.data }),
    [tasksQuery.data],
  )

  if (!projectId) return null

  if (tasksQuery.isLoading) return <MetricsSkeleton />
  if (tasksQuery.isError) {
    return (
      <div className="flex items-center gap-2 px-1 py-3 text-xs text-[var(--red)]">
        <AlertCircle size={14} />
        <span>Failed to load metrics.</span>
      </div>
    )
  }

  const { throughput } = metrics

  return (
    <div className="@container flex flex-col gap-8">
      {/* Throughput */}
      <section className="dashboard-section">
        <div className="dashboard-section-heading mb-3">
          <div className="dashboard-section-heading-main">
            <div className="flex items-center gap-2">
              <BarChart3 size={14} className="text-[var(--accent)]" />
              <span className="dashboard-section-title">Throughput</span>
            </div>
            <p className="dashboard-section-caption">Backlog now, and average times across recent work.</p>
          </div>
        </div>
        <div className="grid grid-cols-2 gap-2.5 @min-[680px]:grid-cols-4">
          <StatTile
            label="Backlog"
            value={String(throughput.backlog)}
            sub="queued, awaiting pickup"
            tone={throughput.backlog > 0 ? 'amber' : 'default'}
          />
          <StatTile
            label="Avg pickup latency"
            value={formatCoarseDuration(throughput.avgPickupLatencyMs) ?? '—'}
            sub="queued → claimed"
          />
          <StatTile
            label="Avg work time"
            value={formatCoarseDuration(throughput.avgWorkTimeMs) ?? '—'}
            sub="claimed → done"
          />
          <StatTile
            label="Completed"
            value={String(throughput.completed)}
            sub={`of ${throughput.total} total tasks`}
            tone={throughput.completed > 0 ? 'green' : 'default'}
          />
        </div>
      </section>
    </div>
  )
}
