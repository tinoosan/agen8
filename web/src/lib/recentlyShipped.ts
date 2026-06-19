/**
 * recentlyShipped — pure grouping logic for the "Recently shipped" card.
 *
 * Factored out of the component so it can be unit-tested without a React tree.
 * The component feeds live query data into this function; the tests pass
 * synthetic fixtures.
 *
 * Design: dec-7ce5a226.
 */

import type { Task, MissionView, KeyResultView } from './types'

export interface ShippedTask {
  task: Task
  /** Display label for the agent who did the work. */
  agentLabel: string
  /** ISO string from task.completedAt, guaranteed to be present. */
  completedAt: string
}

export interface ShippedMissionGroup {
  missionId: string | null
  missionTitle: string
  /** Tasks shown, capped to MAX_TASKS_PER_GROUP. */
  tasks: ShippedTask[]
  /** How many tasks in this group were trimmed beyond the per-group cap (≥0). */
  moreTasksCount: number
  /** ISO string of the most-recent completedAt in the group. */
  latestAt: string
}

export interface RecentlyShippedResult {
  /** Top mission groups, already capped to MAX_MISSION_GROUPS. */
  groups: ShippedMissionGroup[]
  /** How many mission groups were omitted beyond the cap. */
  moreGroupsCount: number
  /** Total succeeded tasks in the window across ALL missions, before any cap. */
  totalShipped: number
}

/** Cap how many mission groups we surface. */
const MAX_MISSION_GROUPS = 6
/** Cap how many tasks we show per expanded group. */
const MAX_TASKS_PER_GROUP = 5

/**
 * groupRecentlyShipped derives "recently shipped" task groups.
 *
 * @param tasks         Full task list from useProjectTasks.
 * @param missions      Full mission list (all statuses) from useMissions.
 * @param krMap         KR map from useProjectKRs (keyed by kr.id and variants).
 * @param nowMs         Current time in ms (Date.now()). Injected for testability.
 * @param windowMs      Look-back window in ms. Defaults to 48 h.
 *
 * Mission resolution:
 *   1. task.missionRef   — direct link (preferred)
 *   2. task.keyResultRef → KR.missionId — indirect via the KR map
 *   3. Unresolvable → group key null, title "No mission"
 *
 * Grouping: one entry per mission (or the null sentinel).
 * Sort: by most-recent completedAt in the group, descending.
 * Cap: top MAX_MISSION_GROUPS groups; within each group, top MAX_TASKS_PER_GROUP tasks.
 *
 * Returns null when the window contains no succeeded tasks.
 */
export function groupRecentlyShipped(
  tasks: Task[],
  missions: MissionView[],
  krMap: Map<string, KeyResultView>,
  nowMs: number,
  windowMs: number = 48 * 60 * 60 * 1000,
): RecentlyShippedResult | null {
  const since = nowMs - windowMs

  // Build a fast lookup from mission id → title.
  const missionTitleById = new Map<string, string>()
  for (const m of missions) {
    missionTitleById.set(m.id, m.title)
  }

  // Filter to succeeded tasks within the window.
  const relevant = tasks.filter((t) => {
    if (t.status !== 'succeeded') return false
    if (!t.completedAt) return false
    return new Date(t.completedAt).getTime() > since
  })

  if (relevant.length === 0) return null

  // Group by resolved mission id (null = no mission).
  const groupMap = new Map<string | null, ShippedTask[]>()

  for (const t of relevant) {
    // Resolve mission id.
    let missionId: string | null = null

    if (t.missionRef) {
      // Accept bare ids and "mission:<id>" prefixed refs.
      const ref = t.missionRef.startsWith('mission:')
        ? t.missionRef.slice('mission:'.length)
        : t.missionRef
      if (missionTitleById.has(ref)) {
        missionId = ref
      }
    }

    if (!missionId && t.keyResultRef) {
      const kr = krMap.get(t.keyResultRef)
      if (kr && missionTitleById.has(kr.missionId)) {
        missionId = kr.missionId
      }
    }

    // Build the agent label from task member fields.
    const agentLabel =
      t.claimedByMemberLabel ??
      t.claimedByMemberId ??
      t.assignedToLabel ??
      t.assignedTo ??
      'Unknown'

    const shipped: ShippedTask = {
      task: t,
      agentLabel,
      completedAt: t.completedAt!,
    }

    const existing = groupMap.get(missionId)
    if (existing) {
      existing.push(shipped)
    } else {
      groupMap.set(missionId, [shipped])
    }
  }

  // Build the groups array, sorted within each group by completedAt desc.
  const groups: ShippedMissionGroup[] = []
  for (const [missionId, shippedTasks] of groupMap.entries()) {
    shippedTasks.sort(
      (a, b) => new Date(b.completedAt).getTime() - new Date(a.completedAt).getTime(),
    )
    const latestAt = shippedTasks[0].completedAt
    const missionTitle =
      missionId !== null
        ? (missionTitleById.get(missionId) ?? missionId)
        : 'No mission'

    groups.push({
      missionId,
      missionTitle,
      // Cap tasks per group; surface the trimmed count so the component can show
      // a "+N more" link rather than silently dropping work.
      tasks: shippedTasks.slice(0, MAX_TASKS_PER_GROUP),
      moreTasksCount: Math.max(0, shippedTasks.length - MAX_TASKS_PER_GROUP),
      latestAt,
    })
  }

  // Sort groups by most-recent task activity, descending.
  // The "No mission" null-key group sorts naturally (it has a real latestAt too).
  groups.sort(
    (a, b) => new Date(b.latestAt).getTime() - new Date(a.latestAt).getTime(),
  )

  const moreGroupsCount = Math.max(0, groups.length - MAX_MISSION_GROUPS)
  const cappedGroups = groups.slice(0, MAX_MISSION_GROUPS)

  return {
    groups: cappedGroups,
    moreGroupsCount,
    totalShipped: relevant.length,
  }
}

export { MAX_TASKS_PER_GROUP, MAX_MISSION_GROUPS }
