/**
 * missionProgress — shared mission progress math.
 *
 * A mission's progress is the average of its live (non-dropped) key results'
 * progressPercent, clamped to 0..100. Extracted from the sidebar so the sidebar
 * MissionSection and the Missions page compute identical numbers from the same
 * rule, and so it can be unit-tested without a React tree.
 */

import type { KeyResultView, MissionStatus } from './types'

export interface MissionProgressSummary {
  /** Rounded average progress across live KRs, 0..100. */
  pct: number
  /** Count of KRs with status 'completed'. */
  completed: number
  /** Count of live (non-dropped) KRs. */
  total: number
}

/**
 * keyResultProgressSummary derives a mission's headline progress from its KRs.
 * Dropped KRs are excluded entirely. Returns null when there are no live KRs,
 * so callers can choose to render nothing rather than a misleading 0%.
 */
export function keyResultProgressSummary(
  krs: KeyResultView[] | undefined,
): MissionProgressSummary | null {
  const liveKRs = (krs ?? []).filter((kr) => kr.status !== 'dropped')
  if (liveKRs.length === 0) return null

  const progress = liveKRs.reduce((sum, kr) => {
    const value = Number.isFinite(kr.progressPercent) ? kr.progressPercent : 0
    return sum + Math.min(100, Math.max(0, value))
  }, 0)
  const pct = Math.round(progress / liveKRs.length)
  const completed = liveKRs.filter((kr) => kr.status === 'completed').length

  return { pct, completed, total: liveKRs.length }
}

/** Status-keyed accent used for mission progress bars and icons. */
export function missionProgressColor(status: MissionStatus): string {
  switch (status) {
    case 'active':
      return 'var(--accent)'
    case 'completed':
      return 'var(--green)'
    case 'paused':
      return 'var(--amber)'
    default:
      return 'var(--text-3)'
  }
}
