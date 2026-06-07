import { useMemo, useState } from 'react'
import { Link } from 'wouter'
import {
  Activity,
  Plus,
  Play,
  Check,
  XCircle,
  Ban,
  FileText,
  Target,
  AlertCircle,
  type LucideIcon,
} from 'lucide-react'
import { cn } from '@/lib/utils'
import { Skeleton } from '@/components/ui/skeleton'
import { formatRelative } from '@/lib/format'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useMissions } from '../../hooks/useMissions'
import {
  buildActivityEvents,
  groupActivityByBucket,
  type ActivityEvent,
  type ActivityKind,
  type ActivityType,
} from '../../lib/activityFeed'

/* ── Presentation maps ──────────────────────────────────────────────────
 * Verb / icon / color are PRESENTATION, so they live here in the component
 * rather than in the pure projection lib. Each event type gets a past-tense
 * verb (GitHub-timeline voice), a lucide icon for its node, and a token
 * color that matches the app's status palette. */

const VERB: Record<ActivityType, string> = {
  'task.created': 'created task',
  'task.started': 'started working on',
  'task.completed': 'completed task',
  'task.failed': 'failed task',
  'task.canceled': 'canceled task',
  'decision.logged': 'logged a decision',
  'mission.created': 'created mission',
}

const ICON: Record<ActivityType, LucideIcon> = {
  'task.created': Plus,
  'task.started': Play,
  'task.completed': Check,
  'task.failed': XCircle,
  'task.canceled': Ban,
  'decision.logged': FileText,
  'mission.created': Target,
}

const COLOR: Record<ActivityType, string> = {
  'task.created': 'var(--text-3)',
  'task.started': 'var(--blue)',
  'task.completed': 'var(--green)',
  'task.failed': 'var(--red)',
  'task.canceled': 'var(--text-3)',
  'decision.logged': 'var(--purple)',
  'mission.created': 'var(--accent)',
}

/* ── Filter chips ────────────────────────────────────────────────────── */

type FilterKey = 'all' | ActivityKind

const FILTERS: { key: FilterKey; label: string }[] = [
  { key: 'all', label: 'All' },
  { key: 'task', label: 'Tasks' },
  { key: 'decision', label: 'Decisions' },
  { key: 'mission', label: 'Missions' },
]

function FilterChips({
  active,
  counts,
  onChange,
}: {
  active: FilterKey
  counts: Record<FilterKey, number>
  onChange: (k: FilterKey) => void
}) {
  return (
    <div className="flex flex-wrap items-center gap-2" role="tablist" aria-label="Filter activity by kind">
      {FILTERS.map(({ key, label }) => {
        const on = active === key
        return (
          <button
            key={key}
            type="button"
            role="tab"
            aria-selected={on}
            onClick={() => onChange(key)}
            className={cn(
              'inline-flex items-center gap-1.5 rounded-full border px-3 py-1 text-[0.75rem] font-medium transition-colors',
              on
                ? 'border-[var(--border-strong)] bg-[var(--bg-surface)] text-[var(--text-1)]'
                : 'border-[var(--border)] bg-[var(--bg-elevated)] text-[var(--text-2)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)]',
            )}
          >
            {label}
            <span className="text-[0.625rem] tabular-nums text-[var(--text-3)]">{counts[key]}</span>
          </button>
        )
      })}
    </div>
  )
}

/* ── A single timeline row ───────────────────────────────────────────── */

function ActivityRow({ event }: { event: ActivityEvent }) {
  const Icon = ICON[event.type]
  const color = COLOR[event.type]
  return (
    <div className="activity-ev animate-fade-in">
      <div className="activity-node" style={{ color }}>
        <Icon size={15} aria-hidden />
      </div>
      <div className="min-w-0 flex-1 pt-[5px]">
        <div className="text-[0.8125rem] leading-[1.4] text-[var(--text-2)]">
          {event.actor && <span className="font-semibold text-[var(--text-1)]">{event.actor} </span>}
          <span>{VERB[event.type]} </span>
          <Link to={event.link} className="font-medium text-[var(--text-1)] hover:underline">
            {event.subject}
          </Link>
        </div>
        <div className="mt-0.5 flex items-center gap-2 text-[0.6875rem] tabular-nums text-[var(--text-3)]">
          <span>{formatRelative(event.at, { seconds: true })}</span>
        </div>
      </div>
    </div>
  )
}

/* ── Loading skeleton ────────────────────────────────────────────────── */

function ActivityFeedSkeleton() {
  return (
    <div className="flex flex-col gap-3">
      {[1, 2, 3, 4, 5].map(i => (
        <div key={i} className="flex items-center gap-3.5 py-1.5">
          <Skeleton className="h-8 w-8 rounded-full" />
          <div className="flex-1">
            <Skeleton className="mb-1.5 h-3.5 w-64" />
            <Skeleton className="h-3 w-24" />
          </div>
        </div>
      ))}
    </div>
  )
}

/* ── Main exported component ─────────────────────────────────────────── */

export default function ActivityFeed({ projectId }: { projectId: string | null }) {
  const tasksQuery = useProjectTasks(projectId)
  const decisionsQuery = useRecentDecisions(projectId)
  const missionsQuery = useMissions(projectId)

  const [filter, setFilter] = useState<FilterKey>('all')

  const allEvents = useMemo(
    () =>
      buildActivityEvents({
        projectId: projectId ?? '',
        tasks: tasksQuery.data,
        decisions: decisionsQuery.data,
        missions: missionsQuery.data,
      }),
    [projectId, tasksQuery.data, decisionsQuery.data, missionsQuery.data],
  )

  const counts = useMemo<Record<FilterKey, number>>(() => {
    const c: Record<FilterKey, number> = { all: allEvents.length, task: 0, decision: 0, mission: 0 }
    for (const e of allEvents) c[e.kind] += 1
    return c
  }, [allEvents])

  const visible = useMemo(
    () => (filter === 'all' ? allEvents : allEvents.filter(e => e.kind === filter)),
    [allEvents, filter],
  )

  const groups = useMemo(() => groupActivityByBucket(visible), [visible])

  if (!projectId) return null

  const isLoading = tasksQuery.isLoading || decisionsQuery.isLoading || missionsQuery.isLoading
  const isError = tasksQuery.isError || decisionsQuery.isError || missionsQuery.isError

  return (
    <section className="dashboard-section">
      {/* Heading */}
      <div className="dashboard-section-heading mb-4">
        <div className="dashboard-section-heading-main">
          <div className="flex items-center gap-2">
            <Activity size={14} className="text-[var(--accent)]" />
            <span className="dashboard-section-title">Activity</span>
          </div>
          <p className="dashboard-section-caption">What agents have done, newest first.</p>
        </div>
      </div>

      {/* Filters */}
      {!isLoading && !isError && allEvents.length > 0 && (
        <div className="mb-5">
          <FilterChips active={filter} counts={counts} onChange={setFilter} />
        </div>
      )}

      {/* Body */}
      {isLoading ? (
        <ActivityFeedSkeleton />
      ) : isError ? (
        <div className="flex items-center gap-2 px-1 py-3 text-xs text-[var(--red)]">
          <AlertCircle size={14} />
          <span>Failed to load activity.</span>
        </div>
      ) : allEvents.length === 0 ? (
        <div className="flex items-center gap-2.5 px-1 py-6 text-[0.8125rem] text-[var(--text-3)]">
          <Activity size={16} className="opacity-40" />
          <span>No activity yet — milestones will appear here as agents work.</span>
        </div>
      ) : visible.length === 0 ? (
        <div className="px-1 py-6 text-[0.8125rem] text-[var(--text-3)]">
          Nothing in this filter.
        </div>
      ) : (
        <div>
          {groups.map(({ bucket, items }) => (
            <div key={bucket}>
              <div className="decision-time-divider">{bucket}</div>
              <div className="activity-feed">
                {items.map(event => (
                  <ActivityRow key={event.id} event={event} />
                ))}
              </div>
            </div>
          ))}
        </div>
      )}
    </section>
  )
}
