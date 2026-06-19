/**
 * missionProgress — shared mission progress math.
 *
 * A mission's progress is the average of its live (non-dropped) key results'
 * progressPercent, clamped to 0..100. Extracted from the sidebar so the sidebar
 * MissionSection and the Missions page compute identical numbers from the same
 * rule, and so it can be unit-tested without a React tree.
 */

import type { KeyResultView, MissionStatus, MissionView } from './types'

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

/**
 * groupKRsByMission groups KRs by missionId, deduping by KR id.
 *
 * useProjectKRs returns a Map keyed by both the raw id and prefixed variants, so
 * iterating its values yields each KR more than once — the dedupe set keeps the
 * grouping honest. Accepts any iterable of KRs (the map's values or a plain list).
 */
export function groupKRsByMission(krs: Iterable<KeyResultView>): Map<string, KeyResultView[]> {
  const byMission = new Map<string, KeyResultView[]>()
  const seen = new Set<string>()
  for (const kr of krs) {
    if (seen.has(kr.id)) continue
    seen.add(kr.id)
    const arr = byMission.get(kr.missionId)
    if (arr) arr.push(kr)
    else byMission.set(kr.missionId, [kr])
  }
  return byMission
}

export interface MissionsOverview {
  total: number
  active: number
  completed: number
  /** Average progress across active missions that have live KRs, or null if none. */
  avgActiveProgress: number | null
  /** Active missions with at least one at-risk KR — the "needs a look" signal. */
  attentionCount: number
  /** Total at-risk KRs across active missions. */
  atRiskKRs: number
}

/**
 * summarizeMissions derives the aggregate tiles for the Missions page from the
 * mission list and KRs grouped by mission. Aggregates focus on ACTIVE work:
 * average progress and at-risk signals only consider active missions, since a
 * completed or archived mission's progress is not actionable.
 */
export function summarizeMissions(
  missions: MissionView[],
  krByMission: Map<string, KeyResultView[]>,
): MissionsOverview {
  let active = 0
  let completed = 0
  let activeProgressSum = 0
  let activeWithKRs = 0
  let atRiskKRs = 0
  let attentionCount = 0

  for (const m of missions) {
    if (m.status === 'completed') completed += 1
    if (m.status !== 'active') continue

    active += 1
    const krs = krByMission.get(m.id) ?? []
    const summary = keyResultProgressSummary(krs)
    if (summary) {
      activeProgressSum += summary.pct
      activeWithKRs += 1
    }
    const riskCount = krs.filter((kr) => kr.status === 'at_risk').length
    atRiskKRs += riskCount
    if (riskCount > 0) attentionCount += 1
  }

  return {
    total: missions.length,
    active,
    completed,
    avgActiveProgress: activeWithKRs > 0 ? Math.round(activeProgressSum / activeWithKRs) : null,
    attentionCount,
    atRiskKRs,
  }
}
