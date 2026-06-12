import { useMemo, useState } from 'react'
import { Link } from 'wouter'
import { useMissions } from '../../hooks/useMissions'
import ListPager, { pageSlice } from '../ListPager'
import { usePins } from '../../hooks/usePins'
import CreateMissionDialog from '../mission/CreateMissionDialog'
import type { MissionStatus } from '../../lib/types'
import { missionDetailLink } from '../../lib/routing'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { cn } from '@/lib/utils'
import { Target, Plus, AlertCircle, Search, Pin, ExternalLink } from 'lucide-react'

type StatusFilter = 'all' | MissionStatus

const STATUS_FILTERS: { value: StatusFilter; label: string }[] = [
  { value: 'all', label: 'All' },
  { value: 'draft', label: 'Draft' },
  { value: 'active', label: 'Active' },
  { value: 'paused', label: 'Paused' },
  { value: 'completed', label: 'Completed' },
  { value: 'archived', label: 'Archived' },
]

function MissionsSkeleton({ embedded = false }: { embedded?: boolean }) {
  return (
    <div className={cn('flex flex-col gap-0.5', embedded && 'px-1')}>
      {[1, 2, 3].map((i) => (
        <div key={i} className="flex items-center gap-3 px-2 py-3">
          <Skeleton className="h-3.5 w-3.5 rounded-sm shrink-0" />
          <Skeleton className="h-3.5 flex-1 max-w-[240px]" />
          <Skeleton className="h-3.5 w-20 ml-auto" />
          <Skeleton className="h-5 w-14 rounded-full" />
        </div>
      ))}
    </div>
  )
}

interface DashboardMissionsPanelProps {
  projectId: string | null
  focusedProjectRoot: string | null
  embedded?: boolean
}

/* Page size for the mission list — same house pagination as decisions/tasks. */
const MISSIONS_PAGE_SIZE = 30

export default function DashboardMissionsPanel({
  projectId,
  embedded = false,
}: DashboardMissionsPanelProps) {
  const [statusFilter, setStatusFilterState] = useState<StatusFilter>('all')
  const [searchQuery, setSearchQueryState] = useState('')
  const [createDialogOpen, setCreateDialogOpen] = useState(false)
  const [page, setPage] = useState(1)

  // Filter/search changes reset to page 1 — same house pagination as the
  // decisions and tasks panels.
  const setStatusFilter = (value: StatusFilter) => {
    setPage(1)
    setStatusFilterState(value)
  }
  const setSearchQuery = (value: string) => {
    setPage(1)
    setSearchQueryState(value)
  }

  const { data: allMissions, isLoading, isError, error } = useMissions(projectId)
  const { isPinned, togglePin } = usePins(projectId)

  const statusCounts = useMemo(
    () =>
      allMissions
        ? allMissions.reduce<Record<string, number>>((acc, mission) => {
            acc[mission.status] = (acc[mission.status] ?? 0) + 1
            return acc
          }, {})
        : {},
    [allMissions],
  )

  const filteredMissions = useMemo(() => {
    if (!allMissions) return []

    let list = statusFilter === 'all'
      ? allMissions
      : allMissions.filter(mission => mission.status === statusFilter)

    if (searchQuery.trim()) {
      const q = searchQuery.toLowerCase()
      list = list.filter(mission =>
        mission.title.toLowerCase().includes(q) || mission.description?.toLowerCase().includes(q),
      )
    }

    return [...list].sort((a, b) => {
      const ap = isPinned(a.id) ? 0 : 1
      const bp = isPinned(b.id) ? 0 : 1
      return ap - bp
    })
  }, [allMissions, statusFilter, searchQuery, isPinned])

  const totalPages = Math.max(1, Math.ceil(filteredMissions.length / MISSIONS_PAGE_SIZE))
  const visibleMissions = useMemo(() => pageSlice(filteredMissions, page, MISSIONS_PAGE_SIZE), [filteredMissions, page])

  if (!projectId) {
    return (
      <div className="flex flex-col items-center justify-center h-full text-center p-8">
        <Target size={36} className="text-[var(--text-3)] opacity-30 mb-4" />
        <h2 className="text-base font-semibold text-[var(--text-1)] mb-1">No project selected</h2>
        <p className="text-sm text-[var(--text-3)]">Select a project to work with missions.</p>
      </div>
    )
  }

  return (
    <div className="flex h-full min-h-0 flex-col">
      <div className={cn('shrink-0', embedded ? 'px-[var(--dashboard-context-gutter)] pt-5 pb-3 border-b border-[color-mix(in_srgb,var(--border)_56%,transparent)]' : 'px-6 pt-8 pb-2 max-w-4xl mx-auto w-full')}>
        <div className="flex items-start justify-between gap-3">
          <div className="min-w-0">
            <h1
              className="m-0 text-[var(--text-1)]"
              style={{ fontSize: embedded ? '1.1875rem' : '1.75rem', fontWeight: 700, letterSpacing: embedded ? '-0.36px' : '-0.56px', lineHeight: embedded ? 1.18 : 1.14 }}
            >
              Missions
            </h1>
            <p className="m-0 mt-1 text-[var(--text-3)]" style={{ fontSize: embedded ? '0.75rem' : '0.8125rem', letterSpacing: embedded ? '-0.12px' : '-0.08px', lineHeight: 1.45 }}>
              Shape the outcomes, key results, and ownership the rest of the work builds on.
            </p>
          </div>
          <Button
            variant="outline"
            size="sm"
            onClick={() => setCreateDialogOpen(true)}
            style={{ letterSpacing: '-0.12px' }}
            className="dashboard-action-button dashboard-action-button-accent"
          >
            <Plus size={13} className="mr-1" />
            New Mission
          </Button>
        </div>
      </div>

      <div className={cn('shrink-0', embedded ? 'px-[var(--dashboard-context-gutter)] py-3 border-b border-[color-mix(in_srgb,var(--border)_42%,transparent)]' : 'px-6 pt-4 pb-2 max-w-4xl mx-auto w-full')}>
        <div className="flex items-center gap-3">
          <div className="flex flex-wrap items-center gap-x-0.5 gap-y-1.5 flex-1 min-w-0">
            {STATUS_FILTERS.map((filter) => {
              const isActive = statusFilter === filter.value
              const count = filter.value === 'all' ? (allMissions?.length ?? 0) : (statusCounts[filter.value] ?? 0)
              return (
                <button
                  key={filter.value}
                  onClick={() => setStatusFilter(filter.value)}
                  aria-pressed={isActive}
                  className="inline-flex items-center gap-1 border-none cursor-pointer transition-colors duration-150 whitespace-nowrap"
                  style={{
                    padding: embedded ? '4px 10px' : '4px 12px',
                    borderRadius: '980px',
                    fontSize: embedded ? '0.75rem' : '0.8125rem',
                    fontWeight: isActive ? 600 : 400,
                    letterSpacing: '-0.08px',
                    background: isActive ? 'color-mix(in srgb, var(--accent-dim) 18%, var(--bg-panel) 82%)' : 'transparent',
                    color: isActive ? 'var(--text-1)' : 'var(--text-2)',
                  }}
                >
                  {filter.label}
                  {count > 0 && (
                    <span
                      className="tabular-nums"
                      style={{
                        fontSize: '0.6875rem',
                        letterSpacing: '-0.06px',
                        color: isActive ? 'var(--text-3)' : 'var(--text-3)',
                      }}
                    >
                      {count}
                    </span>
                  )}
                </button>
              )
            })}
          </div>
        </div>

        {!isLoading && !isError && allMissions && allMissions.length > 0 && (
          <div className="mt-3 flex items-center gap-2">
            <div className="relative flex-1">
              <Search size={11} className="absolute left-2.5 top-1/2 -translate-y-1/2 text-[var(--text-3)] pointer-events-none" />
              <input
                value={searchQuery}
                onChange={(e) => setSearchQuery(e.target.value)}
                placeholder="Search missions…"
                className={cn(
                  'w-full pl-7 pr-3 py-1.5 rounded-[var(--r-md)] outline-none',
                  'bg-[var(--dashboard-subsurface-bg)] border border-[color-mix(in_srgb,var(--border)_54%,transparent)]',
                  'text-[var(--text-1)] placeholder:text-[var(--text-3)]',
                  'focus:border-[var(--accent)]/40 transition-colors',
                )}
                style={{ fontSize: '0.75rem', letterSpacing: '-0.08px' }}
              />
            </div>
          </div>
        )}
      </div>

      <div className={cn('flex-1 min-h-0 overflow-y-auto', embedded ? 'px-[var(--dashboard-context-gutter)] py-4' : 'px-6 py-2 max-w-4xl mx-auto w-full')}>
        {isLoading && <MissionsSkeleton embedded={embedded} />}

        {isError && (
          <div className="flex items-center gap-2 px-4 py-3 rounded-[8px] bg-[var(--dashboard-subsurface-bg)] text-[0.8125rem] text-[var(--red)]" style={{ letterSpacing: '-0.08px' }}>
            <AlertCircle size={16} />
            <span>Failed to load missions: {error instanceof Error ? error.message : 'Unknown error'}</span>
          </div>
        )}

        {!isLoading && !isError && visibleMissions.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Target size={36} className="text-[var(--text-3)] opacity-25 mb-4" />
            {allMissions && allMissions.length === 0 ? (
              <>
                <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '1.0625rem', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
                  No missions yet
                </h3>
                <p className="text-[var(--text-3)] mb-5 max-w-sm" style={{ fontSize: '0.875rem', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
                  Missions define high-level objectives for your project. Create one to start tracking progress with key results.
                </p>
                <Button
                  onClick={() => setCreateDialogOpen(true)}
                  style={{ letterSpacing: '-0.12px' }}
                  className="dashboard-action-button dashboard-action-button-accent"
                >
                  <Plus size={14} className="mr-1.5" />
                  Create your first mission
                </Button>
              </>
            ) : (
              <>
                <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '1.0625rem', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
                  No results
                </h3>
                <p className="text-[var(--text-3)] mb-5" style={{ fontSize: '0.875rem', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
                  Try a different filter or search term.
                </p>
                <Button variant="secondary" onClick={() => { setStatusFilter('all'); setSearchQuery('') }} style={{ letterSpacing: '-0.12px' }}>
                  Clear filters
                </Button>
              </>
            )}
          </div>
        )}

        {!isLoading && !isError && visibleMissions.length > 0 && (
          <div className="flex flex-col gap-0.5">
            {visibleMissions.map((mission) => {
              const pinned = isPinned(mission.id)
              return (
                <div
                  key={mission.id}
                  className="dashboard-queue-row flex items-center gap-2 px-3 py-2.5 rounded-[var(--r-md)] hover:bg-[var(--bg-hover)] transition-colors"
                >
                  <button
                    onClick={() => togglePin(mission.id, 'mission')}
                    className={cn(
                      'shrink-0 p-0.5 rounded-[var(--r-sm)] transition-colors bg-transparent border-none cursor-pointer',
                      pinned ? 'text-[var(--accent)]' : 'text-[var(--text-3)] hover:text-[var(--text-2)]',
                    )}
                    title={pinned ? 'Unpin mission' : 'Pin mission'}
                    aria-pressed={pinned}
                  >
                    <Pin size={12} className={pinned ? 'fill-current' : ''} />
                  </button>
                  <Link
                    to={missionDetailLink(projectId, mission.id)}
                    className="flex items-center gap-2 flex-1 min-w-0 no-underline"
                  >
                    <span className="text-[0.8125rem] font-semibold text-[var(--text-1)] tracking-[-0.02em] truncate flex-1">
                      {mission.title}
                    </span>
                    <ExternalLink size={11} className="shrink-0 text-[var(--text-3)]" />
                  </Link>
                </div>
              )
            })}
            <div className="mt-3 px-1">
              <ListPager page={page} totalPages={totalPages} onPageChange={setPage} />
            </div>
          </div>
        )}
      </div>

      <CreateMissionDialog
        projectId={projectId}
        open={createDialogOpen}
        onOpenChange={setCreateDialogOpen}
      />
    </div>
  )
}
