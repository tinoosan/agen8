import { useMemo, type ReactNode } from 'react'
import { BarChart3, Award, LayoutGrid, AlertCircle } from 'lucide-react'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useProjectMembers } from '../../hooks/useProjectMembers'
import { formatCoarseDuration } from '../../lib/taskTiming'
import {
  computeMetrics,
  formatSuccessRate,
  type LeaderboardEntry,
} from '../../lib/metrics'

/* The metrics page reads the same two queries the dashboard already uses (tasks
 * + roster) and derives everything client-side via lib/metrics.ts — no metrics
 * RPC or table. It rides the existing SSE invalidation, so the numbers refresh
 * as work moves. See lib/metrics.ts for the projection and its honest-MVP rules.
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

/* ── Leaderboard (responsive card/table, mirrors Members.tsx) ───────────────
 * A 6-column table can't reflow, so narrow containers get stacked cards and
 * wide ones (≥640px) get the table. The switch is a CONTAINER query because the
 * inline sidebar eats ~272px, so the viewport overstates the room available. */

function ShareBar({ value }: { value: number }) {
  const width = Math.max(0, Math.min(100, value))
  return (
    <div className="h-1.5 w-full overflow-hidden rounded-full bg-[var(--bg-elevated)]">
      <span
        className="block h-full rounded-full bg-[var(--accent)]"
        style={{ width: `${width}%` }}
      />
    </div>
  )
}

function SuccessPill({ rate }: { rate: number | null }) {
  if (rate === null) return <span className="text-[var(--text-3)]">—</span>
  // Green at/above 90%, amber 70–90%, red below — mirrors the status palette.
  const color = rate >= 0.9 ? 'var(--green)' : rate >= 0.7 ? 'var(--amber)' : 'var(--red)'
  return (
    <span
      className="inline-block rounded-[6px] px-1.5 py-px text-[0.6875rem] font-semibold tabular-nums"
      style={{ color, background: `color-mix(in srgb, ${color} 16%, transparent)` }}
    >
      {formatSuccessRate(rate)}
    </span>
  )
}

function Leaderboard({
  title,
  caption,
  icon: Icon,
  unitLabel,
  entries,
}: {
  title: string
  caption: string
  icon: typeof Award
  unitLabel: string
  entries: LeaderboardEntry[]
}) {
  const maxDone = entries.reduce((m, e) => Math.max(m, e.done), 0)
  const share = (done: number) => (maxDone > 0 ? (done / maxDone) * 100 : 0)

  return (
    <section className="dashboard-section">
      <div className="dashboard-section-heading mb-3">
        <div className="dashboard-section-heading-main">
          <div className="flex items-center gap-2">
            <Icon size={14} className="text-[var(--accent)]" />
            <span className="dashboard-section-title">{title}</span>
          </div>
          <p className="dashboard-section-caption">{caption}</p>
        </div>
      </div>

      {entries.length === 0 ? (
        <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-6 text-center text-[0.8125rem] text-[var(--text-3)]">
          No completed work attributed to a {unitLabel.toLowerCase()} yet.
        </div>
      ) : (
        <div className="@container">
          {/* Cards (narrow) */}
          <div className="flex flex-col gap-2 @min-[640px]:hidden">
            {entries.map((e, i) => (
              <div
                key={e.key}
                className="flex flex-col gap-2.5 rounded-[var(--r-md)] border border-[var(--border)] bg-[var(--bg-surface)] p-3.5"
              >
                <div className="flex items-center gap-2">
                  <span className="text-[0.75rem] tabular-nums text-[var(--text-3)]">{i + 1}</span>
                  <span className="min-w-0 flex-1 truncate text-[0.8125rem] font-medium text-[var(--text-1)]">
                    {e.key}
                  </span>
                  <SuccessPill rate={e.successRate} />
                </div>
                <ShareBar value={share(e.done)} />
                <div className="flex items-center gap-4 text-[0.6875rem] tabular-nums text-[var(--text-3)]">
                  <span><span className="text-[var(--text-2)]">{e.done}</span> done</span>
                  {e.failed > 0 && <span><span className="text-[var(--text-2)]">{e.failed}</span> failed</span>}
                  <span>{formatCoarseDuration(e.avgWorkTimeMs) ?? '—'} avg</span>
                </div>
              </div>
            ))}
          </div>

          {/* Table (wide) */}
          <div className="hidden overflow-hidden rounded-[var(--r-lg)] border border-[var(--border)] bg-[var(--bg-surface)] @min-[640px]:block">
            <table className="w-full border-collapse">
              <thead>
                <tr className="border-b border-[var(--border)]">
                  <Th className="w-[1%] pr-0">#</Th>
                  <Th>{unitLabel}</Th>
                  <Th align="right">Done</Th>
                  <Th align="right">Success</Th>
                  <Th align="right">Avg work time</Th>
                  <Th className="w-[120px]">Share</Th>
                </tr>
              </thead>
              <tbody>
                {entries.map((e, i) => (
                  <tr
                    key={e.key}
                    className="border-b border-[var(--border)] last:border-0 hover:bg-[var(--bg-hover)]"
                  >
                    <Td className="tabular-nums text-[var(--text-3)]">{i + 1}</Td>
                    <Td className="font-medium text-[var(--text-1)]">{e.key}</Td>
                    <Td align="right" className="tabular-nums">{e.done}</Td>
                    <Td align="right"><SuccessPill rate={e.successRate} /></Td>
                    <Td align="right" className="tabular-nums">{formatCoarseDuration(e.avgWorkTimeMs) ?? '—'}</Td>
                    <Td><ShareBar value={share(e.done)} /></Td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </div>
      )}
    </section>
  )
}

function Th({
  children,
  align = 'left',
  className,
}: {
  children: ReactNode
  align?: 'left' | 'right'
  className?: string
}) {
  return (
    <th
      className={cn(
        'px-4 py-2.5 text-[0.625rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-3)]',
        align === 'right' ? 'text-right' : 'text-left',
        className,
      )}
    >
      {children}
    </th>
  )
}

function Td({
  children,
  align = 'left',
  className,
}: {
  children: ReactNode
  align?: 'left' | 'right'
  className?: string
}) {
  return (
    <td
      className={cn(
        'px-4 py-3 text-[0.8125rem] text-[var(--text-2)]',
        align === 'right' ? 'text-right' : 'text-left',
        className,
      )}
    >
      {children}
    </td>
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
      <Skeleton className="h-40 rounded-[var(--r-lg)]" />
    </div>
  )
}

/* ── Main exported component ─────────────────────────────────────────────── */

export default function MetricsPanel({ projectId }: { projectId: string | null }) {
  const tasksQuery = useProjectTasks(projectId)
  const membersQuery = useProjectMembers(projectId)

  const metrics = useMemo(
    () => computeMetrics({ tasks: tasksQuery.data, members: membersQuery.data }),
    [tasksQuery.data, membersQuery.data],
  )

  if (!projectId) return null

  const isLoading = tasksQuery.isLoading || membersQuery.isLoading
  const isError = tasksQuery.isError || membersQuery.isError

  if (isLoading) return <MetricsSkeleton />
  if (isError) {
    return (
      <div className="flex items-center gap-2 px-1 py-3 text-xs text-[var(--red)]">
        <AlertCircle size={14} />
        <span>Failed to load metrics.</span>
      </div>
    )
  }

  const { throughput, modelLeaderboard, harnessLeaderboard } = metrics

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

      <Leaderboard
        title="Model leaderboard"
        caption="Which model is doing the most, and how reliably."
        icon={Award}
        unitLabel="Model"
        entries={modelLeaderboard}
      />

      <Leaderboard
        title="Harness leaderboard"
        caption="Which harness performs best for this project's work."
        icon={LayoutGrid}
        unitLabel="Harness"
        entries={harnessLeaderboard}
      />
    </div>
  )
}
