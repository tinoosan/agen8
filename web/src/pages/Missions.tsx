import { useMemo, useState } from 'react'
import { Link } from 'wouter'
import {
  Target,
  Plus,
  AlertCircle,
  Search,
  Pin,
  ExternalLink,
  Clock,
  CircleCheck,
  CircleAlert,
  CircleX,
  TrendingUp,
  AlertTriangle,
} from 'lucide-react'
import { useNavigation, missionDetailLink } from '../lib/routing'
import { useMissions, useProjectKRs } from '../hooks/useMissions'
import { usePins } from '../hooks/usePins'
import { usePageParam } from '../hooks/usePageParam'
import { pageSlice } from '../components/listPaging'
import ListPager from '../components/ListPager'
import CreateMissionDialog from '../components/mission/CreateMissionDialog'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import {
  keyResultProgressSummary,
  missionProgressColor,
  groupKRsByMission,
  summarizeMissions,
} from '../lib/missionProgress'
import type { MissionStatus } from '../lib/types'

/*
 * Missions — the project's missions on their own page (formerly a wrapper around
 * the dashboard rail's DashboardMissionsPanel). It leads with aggregate tiles
 * and summarized rows that link to the MissionDetail page; per-mission detail
 * lives there, not here. The list surfaces active work first so the page reads
 * as "the state of my missions", not a flat dump of everything ever created.
 */

type StatusFilter = 'all' | MissionStatus

const STATUS_FILTERS: { value: StatusFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'draft', label: 'Draft' },
  { value: 'active', label: 'Active' },
  { value: 'paused', label: 'Paused' },
  { value: 'completed', label: 'Completed' },
  { value: 'archived', label: 'Archived' },
]

/* Relevance order: live work first, the graveyard last. */
const STATUS_ORDER: Record<MissionStatus, number> = {
  active: 0,
  paused: 1,
  draft: 2,
  completed: 3,
  archived: 4,
}

const MISSIONS_PAGE_SIZE = 30

function MissionStatusIcon({ status }: { status: MissionStatus }) {
  switch (status) {
    case 'active':
      return <Clock size={14} className="shrink-0 text-[var(--accent)]" aria-hidden />
    case 'completed':
      return <CircleCheck size={14} className="shrink-0 text-[var(--green)]" aria-hidden />
    case 'paused':
      return <CircleAlert size={14} className="shrink-0 text-[var(--amber)]" aria-hidden />
    case 'archived':
      return <CircleX size={14} className="shrink-0 text-[var(--text-3)]" aria-hidden />
    default:
      return <Target size={14} className="shrink-0 text-[var(--text-3)]" aria-hidden />
  }
}

/* ── Aggregate tile ───────────────────────────────────── */

function Tile({
  label,
  value,
  sub,
  tone,
  icon: Icon,
  onClick,
  active,
}: {
  label: string
  value: string | number
  sub?: string
  tone: string
  icon: typeof Clock
  onClick?: () => void
  active?: boolean
}) {
  const interactive = Boolean(onClick)
  return (
    <button
      type="button"
      onClick={onClick}
      aria-pressed={interactive ? Boolean(active) : undefined}
      disabled={!interactive}
      className={cn(
        'flex flex-col gap-1.5 rounded-[14px] border bg-[var(--bg-elevated)] px-4 py-3 text-left',
        interactive ? 'cursor-pointer transition-colors hover:bg-[var(--bg-hover)]' : 'cursor-default',
        active ? 'border-[var(--accent)]' : 'border-[var(--border)]',
      )}
    >
      <div className="flex items-center gap-1.5">
        <Icon size={13} style={{ color: tone }} aria-hidden />
        <span className="text-[0.6875rem] font-semibold uppercase tracking-[0.05em] text-[var(--text-3)]">
          {label}
        </span>
      </div>
      <span className="text-[1.5rem] font-semibold leading-none tabular-nums" style={{ color: tone }}>
        {value}
      </span>
      {sub && <span className="text-[0.75rem] text-[var(--text-3)]">{sub}</span>}
    </button>
  )
}

/* ── Page ─────────────────────────────────────────────── */

export default function Missions() {
  const { projectId } = useNavigation()
  const [statusFilter, setStatusFilterState] = useState<StatusFilter>('all')
  const [searchQuery, setSearchQueryState] = useState('')
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [page, setPage] = usePageParam()

  const setStatusFilter = (value: StatusFilter) => {
    setPage(1)
    setStatusFilterState(value)
  }
  const setSearchQuery = (value: string) => {
    setPage(1)
    setSearchQueryState(value)
  }

  const { data: allMissions, isLoading, isError, error } = useMissions(projectId)
  const { data: krMap } = useProjectKRs(projectId)
  const { isPinned, togglePin } = usePins(projectId)

  // One KR query for the whole project, grouped by mission — avoids a per-row
  // fetch across dozens of missions.
  const krByMission = useMemo(
    () => groupKRsByMission(krMap ? krMap.values() : []),
    [krMap],
  )

  const overview = useMemo(
    () => summarizeMissions(allMissions ?? [], krByMission),
    [allMissions, krByMission],
  )

  const statusCounts = useMemo(
    () =>
      (allMissions ?? []).reduce<Record<string, number>>((acc, m) => {
        acc[m.status] = (acc[m.status] ?? 0) + 1
        return acc
      }, {}),
    [allMissions],
  )

  const filteredMissions = useMemo(() => {
    if (!allMissions) return []
    let list =
      statusFilter === 'all' ? allMissions : allMissions.filter((m) => m.status === statusFilter)

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase()
      list = list.filter(
        (m) => m.title.toLowerCase().includes(q) || m.description?.toLowerCase().includes(q),
      )
    }

    // Pinned first, then live work before the graveyard, then most-complete first.
    return [...list].sort((a, b) => {
      const ap = isPinned(a.id) ? 0 : 1
      const bp = isPinned(b.id) ? 0 : 1
      if (ap !== bp) return ap - bp
      const ao = STATUS_ORDER[a.status] ?? 9
      const bo = STATUS_ORDER[b.status] ?? 9
      if (ao !== bo) return ao - bo
      const apct = keyResultProgressSummary(krByMission.get(a.id))?.pct ?? -1
      const bpct = keyResultProgressSummary(krByMission.get(b.id))?.pct ?? -1
      return bpct - apct
    })
  }, [allMissions, statusFilter, searchQuery, isPinned, krByMission])

  const totalPages = Math.max(1, Math.ceil(filteredMissions.length / MISSIONS_PAGE_SIZE))
  const visibleMissions = useMemo(
    () => pageSlice(filteredMissions, page, MISSIONS_PAGE_SIZE),
    [filteredMissions, page],
  )

  if (!projectId) {
    return (
      <div className="flex h-full items-center justify-center p-8">
        <div className="rounded-[var(--r-lg)] border border-dashed border-[var(--border)] p-8 text-center text-[0.8125rem] text-[var(--text-3)]">
          Select a project to view its missions.
        </div>
      </div>
    )
  }

  const hasMissions = (allMissions?.length ?? 0) > 0

  return (
    <div className="h-full overflow-y-auto">
      <div className="mx-auto w-full max-w-[960px] px-6 pt-8 pb-12">
        {/* Header */}
        <div className="mb-5 flex items-start justify-between gap-3">
          <h1 className="m-0 text-[1.75rem] font-bold leading-[1.14] tracking-[-0.03em] text-[var(--text-1)]">
            Missions
          </h1>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setCreateDialogOpen(true)}
            className="dashboard-action-button dashboard-action-button-accent"
          >
            <Plus size={13} className="mr-1" />
            New Mission
          </Button>
        </div>

        {/* Aggregate tiles — the at-a-glance state of the missions */}
        {hasMissions && !isError && (
          <div className="@container mb-6">
            <div className="grid grid-cols-2 gap-3 @min-[640px]:grid-cols-4">
              <Tile
                label="Active"
                value={overview.active}
                tone="var(--accent)"
                icon={Clock}
                onClick={() => setStatusFilter(statusFilter === 'active' ? 'all' : 'active')}
                active={statusFilter === 'active'}
              />
              <Tile
                label="Avg progress"
                value={overview.avgActiveProgress === null ? '—' : `${overview.avgActiveProgress}%`}
                sub="across active"
                tone="var(--blue,var(--accent))"
                icon={TrendingUp}
              />
              <Tile
                label="Completed"
                value={overview.completed}
                tone="var(--green,var(--accent))"
                icon={CircleCheck}
                onClick={() => setStatusFilter(statusFilter === 'completed' ? 'all' : 'completed')}
                active={statusFilter === 'completed'}
              />
              <Tile
                label="Needs attention"
                value={overview.attentionCount}
                sub={
                  overview.atRiskKRs > 0
                    ? `${overview.atRiskKRs} at-risk KR${overview.atRiskKRs === 1 ? '' : 's'}`
                    : 'all on track'
                }
                tone={overview.attentionCount > 0 ? 'var(--amber)' : 'var(--text-3)'}
                icon={AlertTriangle}
              />
            </div>
          </div>
        )}

        {/* Filters + search */}
        {hasMissions && !isError && (
          <div className="mb-4">
            <div className="flex flex-wrap items-center gap-x-1 gap-y-1.5">
              {STATUS_FILTERS.map((filter) => {
                const isActive = statusFilter === filter.value
                const count =
                  filter.value === 'all'
                    ? (allMissions?.length ?? 0)
                    : (statusCounts[filter.value] ?? 0)
                return (
                  <button
                    key={filter.value}
                    onClick={() => setStatusFilter(filter.value)}
                    aria-pressed={isActive}
                    className={cn(
                      'inline-flex items-center gap-1 rounded-full border-none px-3 py-1 text-[0.8125rem] transition-colors',
                      isActive
                        ? 'bg-[color-mix(in_srgb,var(--accent-dim)_18%,var(--bg-panel)_82%)] font-semibold text-[var(--text-1)]'
                        : 'cursor-pointer bg-transparent text-[var(--text-2)] hover:text-[var(--text-1)]',
                    )}
                  >
                    {filter.label}
                    {count > 0 && (
                      <span className="text-[0.6875rem] tabular-nums text-[var(--text-3)]">{count}</span>
                    )}
                  </button>
                )
              })}
            </div>
            <div className="relative mt-3">
              <Search
                size={12}
                className="pointer-events-none absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)]"
              />
              <input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search missions…"
                className="w-full rounded-[var(--r-md)] border border-[color-mix(in_srgb,var(--border)_54%,transparent)] bg-[var(--bg-elevated)] py-1.5 pl-7 pr-3 text-[0.8125rem] text-[var(--text-1)] outline-none transition-colors placeholder:text-[var(--text-3)] focus:border-[var(--accent)]/40"
              />
            </div>
          </div>
        )}

        {/* List */}
        {isLoading && (
          <div className="flex flex-col gap-2">
            {[1, 2, 3, 4].map((i) => (
              <Skeleton key={i} className="h-[3.25rem] rounded-[var(--r-md)]" />
            ))}
          </div>
        )}

        {isError && (
          <div className="flex items-center gap-2 rounded-[8px] bg-[var(--bg-elevated)] px-4 py-3 text-[0.8125rem] text-[var(--red)]">
            <AlertCircle size={16} />
            <span>Failed to load missions: {error instanceof Error ? error.message : 'Unknown error'}</span>
          </div>
        )}

        {!isLoading && !isError && visibleMissions.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Target size={36} className="mb-4 text-[var(--text-3)] opacity-25" />
            {!hasMissions ? (
              <>
                <h3 className="mb-1.5 text-[1.0625rem] font-semibold tracking-[-0.01em] text-[var(--text-1)]">
                  No missions yet
                </h3>
                <p className="mb-5 max-w-sm text-[0.875rem] text-[var(--text-3)]">
                  Missions define high-level objectives. Create one to start tracking progress with key
                  results.
                </p>
                <Button
                  onClick={() => setCreateDialogOpen(true)}
                  className="dashboard-action-button dashboard-action-button-accent"
                >
                  <Plus size={14} className="mr-1.5" />
                  Create your first mission
                </Button>
              </>
            ) : (
              <>
                <h3 className="mb-1.5 text-[1.0625rem] font-semibold tracking-[-0.01em] text-[var(--text-1)]">
                  No results
                </h3>
                <p className="mb-5 text-[0.875rem] text-[var(--text-3)]">
                  Try a different filter or search term.
                </p>
                <Button
                  variant="secondary"
                  onClick={() => {
                    setStatusFilter('all')
                    setSearchQuery('')
                  }}
                >
                  Clear filters
                </Button>
              </>
            )}
          </div>
        )}

        {!isLoading && !isError && visibleMissions.length > 0 && (
          <div className="overflow-hidden rounded-[14px] border border-[var(--border)] bg-[var(--bg-elevated)]">
            {visibleMissions.map((mission, i) => {
              const pinned = isPinned(mission.id)
              const summary = keyResultProgressSummary(krByMission.get(mission.id))
              const color = missionProgressColor(mission.status)
              return (
                <div
                  key={mission.id}
                  className={cn(
                    'group flex items-center gap-3 px-4 py-3 transition-colors hover:bg-[var(--bg-hover)]',
                    i > 0 && 'border-t border-[var(--border)]',
                  )}
                >
                  <button
                    onClick={() => togglePin(mission.id, 'mission')}
                    className={cn(
                      'shrink-0 cursor-pointer rounded-[var(--r-sm)] border-none bg-transparent p-0.5 transition-colors',
                      pinned ? 'text-[var(--accent)]' : 'text-[var(--text-3)] hover:text-[var(--text-2)]',
                    )}
                    title={pinned ? 'Unpin mission' : 'Pin mission'}
                    aria-pressed={pinned}
                  >
                    <Pin size={12} className={pinned ? 'fill-current' : ''} />
                  </button>
                  <MissionStatusIcon status={mission.status} />
                  <Link
                    to={missionDetailLink(projectId, mission.id)}
                    className="flex min-w-0 flex-1 items-center gap-3 no-underline"
                  >
                    <span className="min-w-0 flex-1 truncate text-[0.875rem] font-medium tracking-[-0.01em] text-[var(--text-1)]">
                      {mission.title}
                    </span>
                    {summary ? (
                      <span className="flex shrink-0 items-center gap-2">
                        <span className="hidden text-[0.75rem] tabular-nums text-[var(--text-3)] sm:inline">
                          {summary.pct}% · {summary.total} {summary.total === 1 ? 'KR' : 'KRs'}
                        </span>
                        <span className="h-1 w-[56px] shrink-0 overflow-hidden rounded-full bg-[var(--bg-surface,var(--bg-hover))]">
                          <span
                            className="block h-full rounded-full"
                            style={{ width: `${summary.pct}%`, backgroundColor: color }}
                          />
                        </span>
                      </span>
                    ) : (
                      <span className="hidden shrink-0 text-[0.75rem] text-[var(--text-3)] sm:inline">
                        no KRs
                      </span>
                    )}
                    <ExternalLink
                      size={12}
                      className="shrink-0 text-[var(--text-3)] opacity-0 transition-opacity group-hover:opacity-100"
                    />
                  </Link>
                </div>
              )
            })}
          </div>
        )}

        {!isLoading && !isError && totalPages > 1 && (
          <div className="mt-4">
            <ListPager page={page} totalPages={totalPages} onPageChange={setPage} />
          </div>
        )}
      </div>

      <CreateMissionDialog projectId={projectId} open={createDialogOpen} onOpenChange={setCreateDialogOpen} />
    </div>
  )
}
