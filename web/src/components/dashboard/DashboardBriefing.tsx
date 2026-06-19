/**
 * DashboardBriefing — the hero "vitals" line.
 *
 * A persistent, scannable one-liner directly under the greeting that
 * synthesizes the durable work state: what needs a person, what's in flight,
 * what was completed and decided recently, how many missions are active. It is
 * the briefing headline — the detail cards below are the drill-down.
 *
 * It reuses the queries the dashboard already loads (tasks, decisions, active
 * missions), so it adds no network calls — react-query dedupes by key.
 *
 * "Needs you" is the one actionable signal, so it always renders and is the only
 * chip that draws the eye (amber + tint) when there's something; when the board
 * is clear it reads calm ("Nothing needs you"). Every other chip is shown only
 * when non-zero, so a quiet project shows a quiet line instead of a row of 0s.
 *
 * Copy is domain-neutral — agen8 is a work-context layer for any work, so we
 * count "completed" / "in flight", never "shipped" / "deployed".
 */

import { useEffect, useMemo, useState } from 'react'
import { Link } from 'wouter'
import {
  Activity,
  AlertCircle,
  CircleAlert,
  CircleCheck,
  CircleDashed,
  ScrollText,
  Target,
  type LucideIcon,
} from 'lucide-react'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useMissions } from '../../hooks/useMissions'
import { computeBriefing } from '../../lib/dashboardBriefing'
import { tasksLink, filteredTasksLink, missionsPageLink, decisionsLink } from '../../lib/routing'

/* ── One vitals chip — icon + count + label, links to the detail page ──── */

function Stat({
  to,
  icon: Icon,
  count,
  label,
  title,
  tone,
  emphasis,
}: {
  to: string
  icon: LucideIcon
  /** Rendered before the label. Omitted (null) for the calm "Nothing needs you" chip. */
  count: number | null
  label: string
  title: string
  /** Semantic colour for the icon (and, when emphasised, the count). */
  tone: string
  /** Draw the eye: tint the chip and colour the count. Reserved for "needs you". */
  emphasis?: boolean
}) {
  return (
    <Link
      to={to}
      title={title}
      className={`group inline-flex items-center gap-1.5 rounded-[8px] px-2 py-1 text-[0.8125rem] no-underline transition-colors hover:bg-[var(--bg-hover)] ${
        emphasis ? 'bg-[color-mix(in_srgb,var(--amber)_12%,transparent)]' : ''
      }`}
    >
      <Icon size={14} style={{ color: tone }} aria-hidden />
      {count !== null && (
        <span
          className="font-semibold tabular-nums"
          style={{ color: emphasis ? tone : 'var(--text-1)' }}
        >
          {count}
        </span>
      )}
      <span className="text-[var(--text-3)] group-hover:text-[var(--text-2)]">{label}</span>
    </Link>
  )
}

/* ── Main exported component ──────────────────────────── */

export default function DashboardBriefing({ projectId }: { projectId: string | null }) {
  const tasksQuery = useProjectTasks(projectId)
  const decisionsQuery = useRecentDecisions(projectId)
  const activeMissionsQuery = useMissions(projectId, 'active')

  // The look-back windows ("completed"/"decisions" in 48 h) need the current
  // time, but reading the clock during render is impure. Read it in an effect
  // and refresh on a slow tick so the window stays current over a long-lived
  // session — same shape as NeedsAttention's `now`.
  const [nowMs, setNowMs] = useState<number | null>(null)
  useEffect(() => {
    const updateNow = () => setNowMs(Date.now())
    updateNow()
    const interval = window.setInterval(updateNow, 60_000)
    return () => window.clearInterval(interval)
  }, [])

  const briefing = useMemo(() => {
    if (!tasksQuery.data || nowMs === null) return null
    return computeBriefing(
      tasksQuery.data,
      decisionsQuery.data ?? [],
      activeMissionsQuery.data ?? [],
      nowMs,
    )
  }, [tasksQuery.data, decisionsQuery.data, activeMissionsQuery.data, nowMs])

  if (!projectId) return null

  // The briefing is the topmost consumer of the shared tasks query, so it owns
  // the load-error surface for the task-driven sections below (Working now
  // stays quiet on error, expecting the error to show up here).
  if (tasksQuery.isError) {
    return (
      <div className="mb-8 flex items-center gap-2 px-1 py-2 text-[0.75rem] text-[var(--red)]">
        <AlertCircle size={13} aria-hidden />
        <span>
          Failed to load tasks:{' '}
          {tasksQuery.error instanceof Error ? tasksQuery.error.message : 'Unknown error'}
        </span>
      </div>
    )
  }

  // Hide until the project has tasks at all — so an empty project shows the
  // getting-started card instead of a line of zeros.
  if (!tasksQuery.data || tasksQuery.data.length === 0) return null
  if (!briefing) return null

  const { needsYou, queued, inFlight, completed, decisions, activeMissions } = briefing

  return (
    <nav
      className="mb-8 flex flex-wrap items-center gap-x-1 gap-y-1 -ml-2"
      aria-label="At a glance"
    >
      {/* Needs you — always shown; the only chip that draws the eye when active. */}
      {needsYou > 0 ? (
        <Stat
          to={tasksLink(projectId)}
          icon={CircleAlert}
          count={needsYou}
          label={needsYou === 1 ? 'needs you' : 'need you'}
          title="Tasks waiting on a person (blocked or in review)"
          tone="var(--amber)"
          emphasis
        />
      ) : (
        <Stat
          to={tasksLink(projectId)}
          icon={CircleCheck}
          count={null}
          label="Nothing needs you"
          title="No tasks are blocked or waiting for review"
          tone="var(--text-3)"
        />
      )}

      {inFlight > 0 && (
        <Stat
          to={filteredTasksLink(projectId, 'active')}
          icon={Activity}
          count={inFlight}
          label="in flight"
          title="Tasks an agent is actively working right now"
          tone="var(--accent)"
        />
      )}

      {queued > 0 && (
        <Stat
          to={filteredTasksLink(projectId, 'pending')}
          icon={CircleDashed}
          count={queued}
          label="queued"
          title="Tasks waiting to be picked up"
          tone="var(--text-3)"
        />
      )}

      {completed > 0 && (
        <Stat
          to={filteredTasksLink(projectId, 'succeeded')}
          icon={CircleCheck}
          count={completed}
          label={completed === 1 ? 'completed' : 'completed'}
          title="Tasks completed in the last 48 hours"
          tone="var(--green,var(--accent))"
        />
      )}

      {decisions > 0 && (
        <Stat
          to={decisionsLink(projectId)}
          icon={ScrollText}
          count={decisions}
          label={decisions === 1 ? 'decision' : 'decisions'}
          title="Decisions logged in the last 48 hours"
          tone="var(--accent)"
        />
      )}

      {activeMissions > 0 && (
        <Stat
          to={missionsPageLink(projectId)}
          icon={Target}
          count={activeMissions}
          label={activeMissions === 1 ? 'mission active' : 'missions active'}
          title="Missions currently active"
          tone="var(--text-2)"
        />
      )}
    </nav>
  )
}
