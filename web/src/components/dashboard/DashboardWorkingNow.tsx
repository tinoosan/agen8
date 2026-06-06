import { useMemo } from 'react'
import { Link } from 'wouter'
import { Activity, ChevronRight } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { useProjectTasks, useProjectTasksSSE } from '../../hooks/useProjectTasks'
import { taskClaimedMemberLabel, taskAssignedMemberLabel } from '../../lib/taskMembers'
import { taskStatusColor, taskStatusLabel } from '../../lib/statusLabels'
import { taskDetailLink } from '../../lib/routing'
import type { Task } from '../../lib/types'

/* In-flight = work an agent is actively on, not queued and not finished:
 * active, blocked, and in_review. Pending/terminal states live in the count
 * tiles above (TaskSummary); this strip is the live "who's on what" view. */
const IN_FLIGHT = new Set(['active', 'blocked', 'in_review'])

/* "Atlas (Backend Engineer)" → { base: 'Atlas', role: 'Backend Engineer' } so
 * the name reads bold and the role recedes, instead of one long string. */
function parseAgent(name: string): { base: string; role?: string } {
  const m = name.trim().match(/^(.*?)\s*\((.*)\)\s*$/)
  if (m) return { base: m[1].trim(), role: m[2].trim() }
  return { base: name.trim() }
}

function initials(base: string): string {
  const parts = base.split(/\s+/).filter(Boolean)
  if (parts.length === 0) return '?'
  if (parts.length === 1) return parts[0].slice(0, 2).toUpperCase()
  return (parts[0][0] + parts[parts.length - 1][0]).toUpperCase()
}

/* Stable per-agent accent so two agents read as two people at a glance. */
const AVATAR_TOKENS = ['--accent', '--blue', '--green'] as const
function avatarToken(seed: string): string {
  let h = 0
  for (let i = 0; i < seed.length; i++) h = (h * 31 + seed.charCodeAt(i)) >>> 0
  return AVATAR_TOKENS[h % AVATAR_TOKENS.length]
}

/* Who's on it: prefer the member who claimed the work, fall back to assignee. */
function agentName(task: Task): string {
  return taskClaimedMemberLabel(task) ?? taskAssignedMemberLabel(task) ?? 'Unassigned'
}

function timeAgo(iso?: string): string {
  if (!iso) return ''
  const then = new Date(iso).getTime()
  if (Number.isNaN(then)) return ''
  const secs = Math.max(0, Math.round((Date.now() - then) / 1000))
  if (secs < 60) return 'just now'
  const mins = Math.round(secs / 60)
  if (mins < 60) return `${mins}m ago`
  const hrs = Math.round(mins / 60)
  if (hrs < 24) return `${hrs}h ago`
  return `${Math.round(hrs / 24)}d ago`
}

/* ── Working row — monogram + agent/role + task + status/time + disclosure ── */

function WorkingRow({ projectId, task, first }: { projectId: string; task: Task; first: boolean }) {
  const { base, role } = parseAgent(agentName(task))
  const token = avatarToken(base)
  const color = taskStatusColor(task.status)
  const label = task.title || task.description
  return (
    <div>
      {/* Hairline inset to start under the text, the way iOS Settings lists read. */}
      {!first && <div className="ml-[64px] h-px bg-[var(--border)]" />}
      <Link
        to={taskDetailLink(projectId, task.id)}
        aria-label={`${base} — ${label} (${taskStatusLabel(task.status)}) — open task`}
        className="flex items-center gap-3 px-4 py-3 no-underline transition-colors hover:bg-[var(--bg-hover)]"
      >
        <div
          className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full text-[0.75rem] font-semibold"
          style={{
            background: `color-mix(in srgb, var(${token}) 20%, var(--bg-elevated))`,
            color: `var(${token})`,
          }}
          aria-hidden
        >
          {initials(base)}
        </div>
        <div className="min-w-0 flex-1">
          <div className="truncate text-[0.9375rem] font-semibold tracking-[-0.01em] text-[var(--text-1)]">
            {base}
            {role && (
              <span className="ml-1.5 text-[0.75rem] font-normal text-[var(--text-3)]">{role}</span>
            )}
          </div>
          <div className="truncate text-[0.8125rem] text-[var(--text-3)]">{label}</div>
        </div>
        <div className="flex shrink-0 flex-col items-end gap-0.5">
          <span className="flex items-center gap-1 text-[0.75rem] font-medium" style={{ color }}>
            <span className="h-1.5 w-1.5 rounded-full" style={{ background: color }} />
            <span className="hidden sm:inline">{taskStatusLabel(task.status)}</span>
          </span>
          <span className="hidden text-[0.6875rem] tabular-nums text-[var(--text-3)] sm:inline">
            {timeAgo(task.updatedAt)}
          </span>
        </div>
        <ChevronRight size={16} className="shrink-0 text-[var(--text-3)]" aria-hidden />
      </Link>
    </div>
  )
}

/* ── Main exported component ──────────────────────────── */

export default function DashboardWorkingNow({ projectId }: { projectId: string | null }) {
  useProjectTasksSSE()
  const { data: tasks, isLoading, isError } = useProjectTasks(projectId)

  const working = useMemo(
    () => (tasks ?? []).filter((t) => IN_FLIGHT.has(t.status ?? '')),
    [tasks],
  )

  if (!projectId) return null

  if (isLoading) {
    return (
      <section className="dashboard-section">
        <Heading count={null} />
        <div className="flex max-w-[720px] flex-col gap-px overflow-hidden rounded-[18px] border border-[var(--border)] bg-[var(--bg-elevated)]">
          <Skeleton className="h-[3.75rem] rounded-none" />
          <Skeleton className="h-[3.75rem] rounded-none" />
        </div>
      </section>
    )
  }

  // The board query is shared with TaskSummary, which renders the error message
  // directly above this section — so on failure we stay quiet rather than print
  // the same error twice. Hiding nothing: the user still sees it up there.
  if (isError) return null

  // Hide entirely until the project has tasks at all (mirrors TaskSummary).
  if (!tasks || tasks.length === 0) return null

  return (
    <section className="dashboard-section">
      <Heading count={working.length} />
      {working.length === 0 ? (
        <div className="max-w-[720px] rounded-[18px] border border-[var(--border)] bg-[var(--bg-elevated)] px-4 py-6 text-center text-[0.8125rem] text-[var(--text-3)]">
          Nobody&apos;s mid-task right now.
        </div>
      ) : (
        <div className="max-w-[720px] overflow-hidden rounded-[18px] border border-[var(--border)] bg-[var(--bg-elevated)]">
          {working.map((t, i) => (
            <WorkingRow key={t.id} projectId={projectId} task={t} first={i === 0} />
          ))}
        </div>
      )}
    </section>
  )
}

function Heading({ count }: { count: number | null }) {
  return (
    <div className="dashboard-section-heading mb-2">
      <div className="dashboard-section-heading-main">
        <div className="flex items-center gap-2">
          <Activity size={14} className="text-[var(--accent)]" />
          <span className="dashboard-section-title">Working now</span>
        </div>
        <p className="dashboard-section-caption">Who&apos;s on what, right now.</p>
      </div>
      {count !== null && (
        <div className="dashboard-section-meta">
          <span className="dashboard-section-counter">{count} in flight</span>
        </div>
      )}
    </div>
  )
}
