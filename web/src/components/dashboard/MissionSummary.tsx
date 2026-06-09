import { useState, useMemo } from 'react'
import { Link } from 'wouter'
import { toast } from 'sonner'
import { useMissions, useKeyResults, useUpdateKRProgress } from '../../hooks/useMissions'
import { Skeleton } from '@/components/ui/skeleton'
import { AlertCircle, BarChart2, ChevronDown, ChevronRight, ExternalLink, Target } from 'lucide-react'
import { cn } from '@/lib/utils'
import { missionDetailLink } from '../../lib/routing'
import type { MissionView, KeyResultView, KeyResultStatus } from '../../lib/types'
import { formatKRProgress } from '../../lib/missionUtils'

/* ── Staleness threshold ──────────────────────────────── */

const STALE_MS = 7 * 24 * 60 * 60 * 1000 // 7 days

function isStaleCompleted(mission: MissionView): boolean {
  if (mission.status !== 'completed') return false
  if (!mission.completedAt) return true // no completedAt = treat as stale
  return Date.now() - new Date(mission.completedAt).getTime() > STALE_MS
}

/* ── Helpers ──────────────────────────────────────────── */

function krStatusBadge(status: KeyResultStatus): { tone: string; label: string } {
  switch (status) {
    case 'on_track':  return { tone: 'dashboard-inline-label-success', label: 'On track' }
    case 'at_risk':   return { tone: 'dashboard-inline-label-warning', label: 'At risk' }
    case 'completed': return { tone: 'dashboard-inline-label-accent', label: 'Completed' }
    case 'dropped':   return { tone: 'dashboard-inline-label-critical', label: 'Dropped' }
    default:          return { tone: '', label: 'Open' }
  }
}

/* ── Single KR row ────────────────────────────────────── */

function KeyResultRow({ kr, missionId }: { kr: KeyResultView; missionId: string }) {
  const [showProgress, setShowProgress] = useState(false)
  const [progressValue, setProgressValue] = useState('')
  const [progressNote, setProgressNote] = useState('')
  const updateProgress = useUpdateKRProgress()
  const badge = krStatusBadge(kr.status)
  const isTerminal = kr.status === 'completed' || kr.status === 'dropped'

  function resetProgress() {
    setProgressValue('')
    setProgressNote('')
    setShowProgress(false)
  }

  async function handleReportProgress() {
    const trimmedNote = progressNote.trim()
    if (!trimmedNote) { toast.error('Note is required — explain what you measured'); return }
    const parsedValue = parseFloat(progressValue)
    if (isNaN(parsedValue)) { toast.error('Value must be a valid number'); return }
    try {
      await updateProgress.mutateAsync({ keyResultId: kr.id, missionId, value: parsedValue, note: trimmedNote })
      toast.success('Progress recorded')
      resetProgress()
    } catch (err) {
      toast.error(err instanceof Error ? err.message : 'Failed to record progress')
    }
  }

  return (
    <div className="py-1.5 border-b border-[var(--border)]/40 last:border-0">
      <div className="flex items-center gap-2">
        <span className="flex-1 min-w-0 truncate text-[0.75rem] text-[var(--text-1)]" style={{ letterSpacing: '-0.08px' }}>
          {kr.title}
        </span>
        {kr.progressPercent > 0 && (
          <span className="tabular-nums text-[0.6875rem] font-medium text-[var(--text-2)] shrink-0">
            {Math.round(kr.progressPercent)}%
          </span>
        )}
        <span className={cn('dashboard-inline-label shrink-0', badge.tone)}>
          {badge.label}
        </span>
        {!isTerminal && (
          <button
            className={cn(
              'shrink-0 p-0.5 rounded-[var(--r-sm)] transition-colors',
              showProgress
                ? 'text-[var(--accent)]'
                : 'text-[var(--text-3)] hover:text-[var(--accent)]',
            )}
            onClick={() => setShowProgress(v => !v)}
            title="Report progress"
          >
            <BarChart2 size={11} />
          </button>
        )}
      </div>

      {kr.progressPercent > 0 && (
        <p className="text-[0.625rem] text-[var(--text-3)] tabular-nums mt-0.5 m-0">
          {formatKRProgress(kr)}
        </p>
      )}

      {/* Inline progress form */}
      {showProgress && (
        <div className="mt-1.5 flex flex-col gap-1.5">
          <div className="flex gap-2">
            <input
              type="number"
              placeholder={String(kr.targetValue)}
              value={progressValue}
              onChange={(e) => setProgressValue(e.target.value)}
              className="w-20 px-2 py-1 text-[0.6875rem] bg-[var(--bg-elevated)] border border-[var(--border)] rounded-[var(--r-sm)] outline-none text-[var(--text-1)] tabular-nums"
              autoFocus
            />
            <input
              placeholder="What did you measure?"
              value={progressNote}
              onChange={(e) => setProgressNote(e.target.value)}
              className="flex-1 px-2 py-1 text-[0.6875rem] bg-[var(--bg-elevated)] border border-[var(--border)] rounded-[var(--r-sm)] outline-none text-[var(--text-1)]"
              onKeyDown={(e) => {
                if (e.key === 'Enter') { e.preventDefault(); handleReportProgress() }
                if (e.key === 'Escape') resetProgress()
              }}
            />
          </div>
          <div className="flex items-center gap-2 justify-end">
            <button
              className="text-[0.6875rem] text-[var(--text-3)] hover:text-[var(--text-1)] transition-colors bg-transparent border-none cursor-pointer p-0"
              onClick={resetProgress}
            >
              Cancel
            </button>
            <button
              className="text-[0.6875rem] text-[var(--accent)] font-medium hover:opacity-80 transition-opacity bg-transparent border-none cursor-pointer p-0 disabled:opacity-40"
              onClick={handleReportProgress}
              disabled={updateProgress.isPending || !progressValue.trim() || !progressNote.trim()}
            >
              {updateProgress.isPending ? 'Recording…' : 'Log progress'}
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

/* ── KR list ──────────────────────────────────────────── */

function MissionKeyResults({ missionId }: { missionId: string }) {
  const { data: keyResults, isLoading, isError, error } = useKeyResults(missionId)

  if (isLoading) {
    return (
      <div className="flex flex-col gap-1.5 pt-1">
        <Skeleton className="h-6 rounded-[var(--r-sm)]" />
        <Skeleton className="h-6 rounded-[var(--r-sm)]" />
      </div>
    )
  }

  if (isError) {
    return (
      <div className="flex items-center gap-1.5 py-1.5 text-[0.6875rem] text-[var(--red)]">
        <AlertCircle size={11} />
        <span>{error instanceof Error ? error.message : 'Failed to load'}</span>
      </div>
    )
  }

  if (!keyResults || keyResults.length === 0) {
    return <p className="text-[0.6875rem] text-[var(--text-3)] py-1">No key results defined</p>
  }

  return (
    <div className="overflow-y-auto" style={{ maxHeight: '200px' }}>
      {keyResults.map(kr => (
        <KeyResultRow key={kr.id} kr={kr} missionId={missionId} />
      ))}
    </div>
  )
}

/* ── Single mission row ───────────────────────────────── */

function MissionRow({ mission, projectId }: { mission: MissionView; projectId: string }) {
  const [open, setOpen] = useState(false)
  const { data: keyResults } = useKeyResults(mission.id)

  const overallProgress = keyResults && keyResults.length > 0
    ? keyResults.reduce((sum, kr) => sum + kr.progressPercent, 0) / keyResults.length
    : 0
  const hasKeyResults = keyResults !== undefined && keyResults.length > 0

  return (
    <div className="dashboard-queue-row border-b border-[var(--border)]/40 last:border-0">
      {/* Trigger row */}
      <button
        className="flex items-center gap-2 w-full text-left bg-transparent border-none cursor-pointer px-3 py-2.5 font-[inherit]"
        onClick={() => setOpen(v => !v)}
      >
        {open
          ? <ChevronDown size={11} className="shrink-0 text-[var(--text-3)]" />
          : <ChevronRight size={11} className="shrink-0 text-[var(--text-3)]" />
        }
        <span className="text-[0.8125rem] font-semibold text-[var(--text-1)] tracking-[-0.02em] truncate flex-1">
          {mission.title}
        </span>
        {hasKeyResults && overallProgress > 0 && (
          <span className="text-[0.6875rem] font-semibold tabular-nums text-[var(--text-2)] shrink-0">
            {Math.round(overallProgress)}%
          </span>
        )}
        <Link
          to={missionDetailLink(projectId, mission.id)}
          className="text-[var(--text-3)] hover:text-[var(--accent)] transition-colors shrink-0"
          onClick={(e: React.MouseEvent) => e.stopPropagation()}
          title="View mission details"
        >
          <ExternalLink size={11} />
        </Link>
      </button>

      {/* Expanded KR list */}
      {open && (
        <div className="dashboard-queue-detail px-4 pb-3 pt-1">
          {mission.description && (
            <p className="text-[0.6875rem] text-[var(--text-3)] leading-relaxed mb-1.5 m-0">
              {mission.description}
            </p>
          )}
          <MissionKeyResults missionId={mission.id} />
        </div>
      )}
    </div>
  )
}

/* ── Loading skeleton ─────────────────────────────────── */

function MissionSummarySkeleton() {
  return (
    <div className="dashboard-section">
      <Skeleton className="h-4 w-32 mb-3" />
      <div className="flex flex-col gap-px">
        {[1, 2, 3].map(i => <Skeleton key={i} className="h-8 rounded-[var(--r-sm)]" />)}
      </div>
    </div>
  )
}

/* ── Main exported component ──────────────────────────── */

export default function MissionSummary({ projectId, mode = 'inMotion' }: { projectId: string | null; mode?: 'active' | 'inMotion' }) {
  const { data: allMissions, isLoading, isError, error } = useMissions(projectId, mode === 'active' ? 'active' : undefined)

  // Only show: active/paused/draft + recently completed (within 7 days)
  // Never show: archived, or completed older than 7 days
  const missions = useMemo<MissionView[]>(() => {
    if (!allMissions) return []
    if (mode === 'active') return allMissions.filter(m => m.status === 'active')
    return allMissions.filter(m => m.status !== 'archived' && !isStaleCompleted(m))
  }, [allMissions, mode])

  if (!projectId) return null
  if (isLoading) return <MissionSummarySkeleton />

  if (isError) {
    return (
      <div className="dashboard-section flex items-center gap-2 px-1 py-3 text-[0.6875rem] text-[var(--red)]">
        <AlertCircle size={13} />
        <span>Failed to load missions: {error instanceof Error ? error.message : 'Unknown error'}</span>
      </div>
    )
  }

  if (!missions || missions.length === 0) return null

  const activeCount = missions.filter(m => m.status === 'active').length
  const title = mode === 'active' ? 'Active Missions' : 'Missions in Motion'

  return (
    <section className="dashboard-section">
      <div className="dashboard-section-heading mb-2">
        <div className="dashboard-section-heading-main">
          <div className="flex items-center gap-2">
            <Target size={14} className="text-[var(--accent)]" />
            <span className="dashboard-section-title">
              {title}
            </span>
          </div>
        </div>
        <div className="dashboard-section-meta">
          {activeCount > 0 && (
            <span className="dashboard-section-counter text-[var(--green)]">
              {activeCount} active
            </span>
          )}
          <span className="dashboard-section-counter">
            {missions.length} shown
          </span>
        </div>
      </div>
      <div className="dashboard-list-surface dashboard-list-surface-flat flex flex-col overflow-hidden">
        {missions.map(mission => (
          <MissionRow key={mission.id} mission={mission} projectId={projectId} />
        ))}
      </div>
    </section>
  )
}
