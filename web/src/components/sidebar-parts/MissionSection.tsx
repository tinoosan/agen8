/**
 * MissionSection — sidebar section showing active/paused/completed
 * missions with status icons and KR progress bars.
 */
import { useLocation } from 'wouter'
import { Clock, CircleCheck, CircleAlert, CircleX, Target } from 'lucide-react'
import { useMissions, useKeyResults } from '../../hooks/useMissions'
import { missionDetailLink } from '../../lib/routing'
import { Skeleton } from '@/components/ui/skeleton'
import type { KeyResultView, MissionView } from '../../lib/types'

/* ── Helpers ───────────────────────────────────────── */

function MissionStatusIcon({ status }: { status: MissionView['status'] }) {
  switch (status) {
    case 'active':
      return <Clock size={13} className="shrink-0 text-[var(--accent)]" />
    case 'completed':
      return <CircleCheck size={13} className="shrink-0 text-[var(--green)]" />
    case 'paused':
      return <CircleAlert size={13} className="shrink-0 text-[var(--amber)]" />
    case 'archived':
      return <CircleX size={13} className="shrink-0 text-[var(--text-3)]" />
    default:
      return <Target size={13} className="shrink-0 text-[var(--text-3)]" />
  }
}

function missionProgressColor(status: MissionView['status']): string {
  switch (status) {
    case 'active': return 'var(--accent)'
    case 'completed': return 'var(--green)'
    case 'paused': return 'var(--amber)'
    default: return 'var(--text-3)'
  }
}

function keyResultProgressSummary(krs: KeyResultView[] | undefined) {
  const liveKRs = (krs ?? []).filter(kr => kr.status !== 'dropped')
  if (liveKRs.length === 0) return null

  const progress = liveKRs.reduce((sum, kr) => {
    const value = Number.isFinite(kr.progressPercent) ? kr.progressPercent : 0
    return sum + Math.min(100, Math.max(0, value))
  }, 0)
  const pct = Math.round(progress / liveKRs.length)
  const completed = liveKRs.filter(kr => kr.status === 'completed').length

  return { pct, completed, total: liveKRs.length }
}

function MissionKRProgress({ missionId }: { missionId: string }) {
  const { data: krs } = useKeyResults(missionId)
  const summary = keyResultProgressSummary(krs)
  if (!summary) return null
  const krLabel = summary.total === 1 ? 'KR' : 'KRs'
  return (
    <span
      className="text-[10px] text-[var(--text-3)]"
      title={`${summary.completed} / ${summary.total} ${krLabel} complete`}
    >
      {summary.pct}% · {summary.total} {krLabel}
    </span>
  )
}

function MissionProgressBar({ missionId, color }: { missionId: string; color: string }) {
  const { data: krs } = useKeyResults(missionId)
  const summary = keyResultProgressSummary(krs)
  if (!summary) return null
  return (
    <div className="w-[40px] h-[3px] bg-[var(--bg-elevated)] rounded-[2px] overflow-hidden shrink-0 mt-[6px]">
      <div
        className="h-full rounded-[2px]"
        style={{ width: `${summary.pct}%`, backgroundColor: color }}
      />
    </div>
  )
}

/* ── Section ───────────────────────────────────────── */

export function MissionsSidebarSection({ projectId }: { projectId: string | null }) {
  const { data: missions, isLoading } = useMissions(projectId)
  const [, navigate] = useLocation()

  if (!projectId) return null

  const visibleMissions = (missions ?? [])
    .filter(m => m.status === 'active' || m.status === 'paused' || m.status === 'completed')
    .sort((a, b) => {
      const order = { active: 0, paused: 1, completed: 2, draft: 3, archived: 4 }
      return (order[a.status] ?? 9) - (order[b.status] ?? 9)
    })
    .slice(0, 8)

  if (isLoading) {
    return (
      <div className="px-4 py-2 flex flex-col gap-1">
        {[1, 2].map(i => (
          <Skeleton key={i} className="h-[28px] rounded" />
        ))}
      </div>
    )
  }

  if (visibleMissions.length === 0) {
    return (
      <div className="px-4 py-2 text-[11px] text-[var(--text-3)]">
        No active missions
      </div>
    )
  }

  return (
    <div className="flex flex-col gap-0">
      {visibleMissions.map(mission => {
        const progressColor = missionProgressColor(mission.status)
        return (
          <button
            key={mission.id}
            type="button"
            className="flex items-start gap-2 px-3.5 py-[5px] text-left cursor-pointer border-0 bg-transparent hover:bg-[var(--bg-hover)] transition-colors w-full"
            onClick={() => navigate(missionDetailLink(projectId, mission.id))}
          >
            <MissionStatusIcon status={mission.status} />
            <div className="flex-1 min-w-0">
              <div className="text-[13px] text-[var(--text-2)] truncate leading-tight">
                {mission.title}
              </div>
              <MissionKRProgress missionId={mission.id} />
            </div>
            <MissionProgressBar missionId={mission.id} color={progressColor} />
          </button>
        )
      })}
    </div>
  )
}
