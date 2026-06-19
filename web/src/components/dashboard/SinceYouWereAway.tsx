/**
 * SinceYouWereAway — "Since you were away" dashboard card.
 *
 * Shows a summary of work that happened since the user last visited:
 *   - completed tasks
 *   - new decisions
 *   - changed missions and key results
 *
 * The last-seen marker is read BEFORE it is updated, so the diff spans the
 * correct window. Dismiss → mark-seen → card disappears until newer work
 * arrives. The card hides entirely when the diff is empty.
 *
 * Design: dec-9d274fe0.
 */

import { useMemo, useRef } from 'react'
import { Clock, X } from 'lucide-react'
import { useLastSeen, useMarkSeen } from '../../hooks/useLastSeen'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useMissions, useProjectKRs } from '../../hooks/useMissions'
import { computeDiff, diffIsEmpty } from '../../lib/sinceYouWereAway'
import type { KeyResultView } from '../../lib/types'

export default function SinceYouWereAway({ projectId }: { projectId: string | null }) {
  const lastSeenQuery = useLastSeen(projectId)
  const markSeen = useMarkSeen(projectId)

  // Capture the last-seen value from the first successful response and keep it
  // stable for the lifetime of this render cycle. If we read from
  // lastSeenQuery.data directly after markSeen succeeds, the query re-fetches
  // with the new timestamp and the diff empties before the user sees it.
  const stableLastSeenAt = useRef<string | null>(null)
  if (lastSeenQuery.isSuccess && stableLastSeenAt.current === null) {
    stableLastSeenAt.current = lastSeenQuery.data?.seenAt ?? ''
  }

  const tasksQuery = useProjectTasks(projectId)
  const decisionsQuery = useRecentDecisions(projectId, {}, { refetchInterval: false })
  const missionsQuery = useMissions(projectId) // all statuses
  const krMapQuery = useProjectKRs(projectId)

  const allKRs = useMemo<KeyResultView[]>(() => {
    if (!krMapQuery.data) return []
    // The map has duplicate keys (mission-id and keyResult:id prefixed).
    // Collect unique entries by id.
    const seen = new Set<string>()
    const out: KeyResultView[] = []
    for (const kr of krMapQuery.data.values()) {
      if (!seen.has(kr.id)) {
        seen.add(kr.id)
        out.push(kr)
      }
    }
    return out
  }, [krMapQuery.data])

  const diff = useMemo(
    () =>
      computeDiff(
        stableLastSeenAt.current,
        tasksQuery.data ?? [],
        decisionsQuery.data ?? [],
        missionsQuery.data ?? [],
        allKRs,
      ),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [
      stableLastSeenAt.current,
      tasksQuery.data,
      decisionsQuery.data,
      missionsQuery.data,
      allKRs,
    ],
  )

  if (!projectId) return null
  // Don't render until we know the last-seen value (avoid flash).
  if (!lastSeenQuery.isSuccess) return null
  // Nothing to show.
  if (diffIsEmpty(diff)) return null

  const total =
    diff.completedTasks.length +
    diff.newDecisions.length +
    diff.changedMissions.length +
    diff.changedKeyResults.length

  const handleDismiss = () => {
    stableLastSeenAt.current = new Date().toISOString()
    markSeen.mutate()
  }

  return (
    <div className="mb-8">
      <section className="dashboard-section @container" aria-label="Since you were away">
        <div className="dashboard-section-heading mb-2">
          <div className="dashboard-section-heading-main">
            <div className="flex items-center gap-2">
              <Clock size={14} className="text-[var(--accent)]" aria-hidden />
              <span className="dashboard-section-title">Since you were away</span>
            </div>
          </div>
          <div className="dashboard-section-meta flex items-center gap-3">
            <span className="dashboard-section-counter">
              {total} {total === 1 ? 'update' : 'updates'}
            </span>
            <button
              type="button"
              onClick={handleDismiss}
              aria-label="Dismiss since you were away"
              className="cursor-pointer border-none bg-transparent p-0.5 text-[var(--text-3)] transition-colors hover:text-[var(--text-1)]"
            >
              <X size={14} />
            </button>
          </div>
        </div>

        <div className="max-w-[720px] rounded-[18px] border border-[var(--border)] bg-[var(--bg-elevated)]">
          {diff.completedTasks.length > 0 && (
            <SummaryGroup
              label="Tasks completed"
              count={diff.completedTasks.length}
              items={diff.completedTasks.map((t) => t.title ?? t.description)}
              last={
                diff.newDecisions.length === 0 &&
                diff.changedMissions.length === 0 &&
                diff.changedKeyResults.length === 0
              }
            />
          )}
          {diff.newDecisions.length > 0 && (
            <SummaryGroup
              label="New decisions"
              count={diff.newDecisions.length}
              items={diff.newDecisions.map((d) => d.title)}
              last={diff.changedMissions.length === 0 && diff.changedKeyResults.length === 0}
            />
          )}
          {diff.changedMissions.length > 0 && (
            <SummaryGroup
              label="Mission updates"
              count={diff.changedMissions.length}
              items={diff.changedMissions.map((m) => m.title)}
              last={diff.changedKeyResults.length === 0}
            />
          )}
          {diff.changedKeyResults.length > 0 && (
            <SummaryGroup
              label="Key result updates"
              count={diff.changedKeyResults.length}
              items={diff.changedKeyResults.map((kr) => kr.title)}
              last
            />
          )}
        </div>
      </section>
    </div>
  )
}

const MAX_VISIBLE = 3

function SummaryGroup({
  label,
  count,
  items,
  last,
}: {
  label: string
  count: number
  items: (string | undefined)[]
  last: boolean
}) {
  const visible = items.slice(0, MAX_VISIBLE)
  const overflow = count - visible.length

  return (
    <div className={last ? '' : 'border-b border-[var(--border)]'}>
      <div className="flex items-center gap-2 px-4 pt-3 pb-1">
        <span className="text-[0.6875rem] font-semibold uppercase tracking-[0.04em] text-[var(--text-2)]">
          {label}
        </span>
        {count > 1 && (
          <span className="ml-auto rounded-full bg-[var(--bg-active)] px-1.5 text-[0.6875rem] font-medium tabular-nums text-[var(--text-3)]">
            {count}
          </span>
        )}
      </div>
      {visible.map((item, i) => (
        <div key={i} className="flex items-center gap-2.5 px-4 py-1.5">
          <span className="h-1.5 w-1.5 shrink-0 rounded-full bg-[var(--accent-muted,var(--accent))]" aria-hidden />
          <span className="min-w-0 flex-1 truncate text-[0.8125rem] text-[var(--text-1)]">
            {item ?? '(untitled)'}
          </span>
        </div>
      ))}
      {overflow > 0 && (
        <div className="px-4 pb-2 text-[0.75rem] text-[var(--text-3)]">
          +{overflow} more
        </div>
      )}
    </div>
  )
}
