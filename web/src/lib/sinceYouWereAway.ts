/**
 * sinceYouWereAway — pure diff logic for the "Since you were away" card.
 *
 * Factored out of the component so it can be unit-tested without a React tree.
 * The component feeds live query data into these functions; the tests pass
 * synthetic fixtures.
 */

import type { Task, DecisionView, MissionView, KeyResultView } from './types'

export interface SinceYouWereAwayDiff {
  completedTasks: Task[]
  newDecisions: DecisionView[]
  changedMissions: MissionView[]
  changedKeyResults: KeyResultView[]
}

/**
 * computeDiff derives what changed since `lastSeenAt`.
 *
 * - completedTasks:    tasks whose completedAt > lastSeenAt
 * - newDecisions:      decisions whose createdAt > lastSeenAt
 * - changedMissions:   missions whose updatedAt > lastSeenAt
 * - changedKeyResults: KRs whose updatedAt > lastSeenAt
 *
 * When lastSeenAt is null or empty the diff is always empty: the user has
 * never visited so we have no baseline to diff against.
 */
export function computeDiff(
  lastSeenAt: string | null,
  tasks: Task[],
  decisions: DecisionView[],
  missions: MissionView[],
  keyResults: KeyResultView[],
): SinceYouWereAwayDiff {
  const empty: SinceYouWereAwayDiff = {
    completedTasks: [],
    newDecisions: [],
    changedMissions: [],
    changedKeyResults: [],
  }

  if (!lastSeenAt) return empty

  const since = new Date(lastSeenAt).getTime()
  if (Number.isNaN(since)) return empty

  return {
    completedTasks: tasks.filter((t) => {
      // The backend TaskStatus terminal value is "succeeded"; completedAt is
      // set when the task moves to a terminal state (succeeded or failed).
      // We only surface the "good" outcome here.
      if (t.status !== 'succeeded') return false
      if (!t.completedAt) return false
      return new Date(t.completedAt).getTime() > since
    }),
    newDecisions: decisions.filter((d) => new Date(d.createdAt).getTime() > since),
    changedMissions: missions.filter((m) => new Date(m.updatedAt).getTime() > since),
    changedKeyResults: keyResults.filter((kr) => new Date(kr.updatedAt).getTime() > since),
  }
}

/** Returns true when the diff has at least one item — drives card visibility. */
export function diffIsEmpty(diff: SinceYouWereAwayDiff): boolean {
  return (
    diff.completedTasks.length === 0 &&
    diff.newDecisions.length === 0 &&
    diff.changedMissions.length === 0 &&
    diff.changedKeyResults.length === 0
  )
}
