/**
 * RecentlyShipped — "Recently shipped" dashboard card.
 *
 * Groups tasks that succeeded in the last 48 h by mission.
 * Each group shows "Mission title — N shipped", expandable to the individual
 * task title + agent + relative time. Hidden when the window is empty.
 *
 * Design: dec-7ce5a226.
 */

import { useMemo, useState } from 'react'
import { Link } from 'wouter'
import { CheckCircle, ChevronDown, ChevronRight } from 'lucide-react'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useMissions, useProjectKRs } from '../../hooks/useMissions'
import { groupRecentlyShipped } from '../../lib/recentlyShipped'
import { filteredTasksLink } from '../../lib/routing'
import { formatRelative } from '@/lib/format'
import type { ShippedMissionGroup } from '../../lib/recentlyShipped'

/* ── Mission group row — collapsible ──────────────────── */

function MissionGroup({
  group,
  first,
  projectId,
}: {
  group: ShippedMissionGroup
  first: boolean
  projectId: string
}) {
  const [open, setOpen] = useState(false)
  // True count for this mission = shown + trimmed, so the header never undercounts.
  const totalInGroup = group.tasks.length + group.moreTasksCount

  return (
    <div className={first ? '' : 'border-t border-[var(--border)]'}>
      <button
        type="button"
        onClick={() => setOpen((v) => !v)}
        className="flex w-full cursor-pointer items-center gap-2 border-none bg-transparent px-4 py-3 text-left transition-colors hover:bg-[var(--bg-hover)]"
        aria-expanded={open}
      >
        <span className="min-w-0 flex-1 truncate text-[0.8125rem] font-medium text-[var(--text-1)]">
          {group.missionTitle}
        </span>
        <span className="shrink-0 text-[0.75rem] tabular-nums text-[var(--text-3)]">
          {totalInGroup} done
        </span>
        <span className="shrink-0 text-[var(--text-3)]" aria-hidden>
          {open ? <ChevronDown size={14} /> : <ChevronRight size={14} />}
        </span>
      </button>

      {open && (
        <div className="pb-2">
          {group.tasks.map((st) => {
            const label = st.task.title || st.task.description
            return (
              <div
                key={st.task.id}
                className="flex items-start gap-2.5 px-4 py-1.5"
              >
                <span
                  className="mt-1 h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--green,var(--accent))]"
                  aria-hidden
                />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-[0.8125rem] text-[var(--text-1)]">
                    {label ?? '(untitled)'}
                  </div>
                  <div className="flex items-center gap-1.5 text-[0.75rem] text-[var(--text-3)]">
                    <span className="truncate">{st.agentLabel}</span>
                    <span aria-hidden>·</span>
                    <span className="shrink-0">{formatRelative(st.completedAt)}</span>
                  </div>
                </div>
              </div>
            )
          })}
          {group.moreTasksCount > 0 && (
            <Link
              to={filteredTasksLink(projectId, 'succeeded')}
              className="flex items-center gap-1 px-4 py-1.5 pl-[1.625rem] text-[0.75rem] text-[var(--text-3)] no-underline transition-colors hover:text-[var(--text-1)]"
            >
              +{group.moreTasksCount} more {group.moreTasksCount === 1 ? 'task' : 'tasks'}
              <ChevronRight size={12} aria-hidden />
            </Link>
          )}
        </div>
      )}
    </div>
  )
}

/* ── Main exported component ──────────────────────────── */

export default function RecentlyShipped({ projectId }: { projectId: string | null }) {
  const tasksQuery = useProjectTasks(projectId)
  const missionsQuery = useMissions(projectId)
  const krMapQuery = useProjectKRs(projectId)

  const result = useMemo(() => {
    if (!tasksQuery.data || !missionsQuery.data) return null
    return groupRecentlyShipped(
      tasksQuery.data,
      missionsQuery.data,
      krMapQuery.data ?? new Map(),
      Date.now(),
    )
  }, [tasksQuery.data, missionsQuery.data, krMapQuery.data])

  if (!projectId) return null
  if (!result) return null

  const { groups, moreGroupsCount, totalShipped } = result

  return (
    <div className="mb-8">
      <section className="dashboard-section @container" aria-label="Recently completed">
        <div className="dashboard-section-heading mb-2">
          <div className="dashboard-section-heading-main">
            <div className="flex items-center gap-2">
              <CheckCircle size={14} className="text-[var(--green,var(--accent))]" aria-hidden />
              <span className="dashboard-section-title">Recently completed</span>
            </div>
          </div>
          <div className="dashboard-section-meta">
            <span className="dashboard-section-counter">
              {totalShipped} {totalShipped === 1 ? 'task' : 'tasks'} in 48 h
            </span>
          </div>
        </div>

        <div className="max-w-[720px] overflow-hidden rounded-[18px] border border-[var(--border)] bg-[var(--bg-elevated)]">
          {groups.map((group, i) => (
            <MissionGroup key={group.missionId ?? '__no_mission__'} group={group} first={i === 0} projectId={projectId} />
          ))}

          {moreGroupsCount > 0 && (
            <div className="border-t border-[var(--border)]">
              <Link
                to={filteredTasksLink(projectId, 'succeeded')}
                className="flex w-full items-center gap-1.5 px-4 py-3 text-[0.8125rem] text-[var(--text-3)] no-underline transition-colors hover:bg-[var(--bg-hover)] hover:text-[var(--text-1)]"
              >
                +{moreGroupsCount} more {moreGroupsCount === 1 ? 'mission' : 'missions'}
                <ChevronRight size={13} aria-hidden />
              </Link>
            </div>
          )}
        </div>
      </section>
    </div>
  )
}
