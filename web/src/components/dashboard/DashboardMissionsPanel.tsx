import { useMemo, useState } from 'react'
import { useMissions, useProjectKRs } from '../../hooks/useMissions'
import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { usePinnedMissions } from '../../hooks/usePinnedMissions'
import MissionEditor from '../mission/MissionEditor'
import CreateMissionDialog from '../mission/CreateMissionDialog'
import type { MissionStatus } from '../../lib/types'
import { spaceSummaryLabel } from '../../lib/spaceOwnerLabels'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from '@/components/ui/dropdown-menu'
import { cn } from '@/lib/utils'
import { Target, Plus, AlertCircle, Search, ChevronsUpDown, Users, ChevronDown } from 'lucide-react'

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

export default function DashboardMissionsPanel({
  projectId,
  embedded = false,
}: DashboardMissionsPanelProps) {
  const [statusFilter, setStatusFilter] = useState<StatusFilter>('all')
  const [searchQuery, setSearchQuery] = useState('')
  const [spaceFilter, setSpaceFilter] = useState<string>('')
  const [expandedIds, setExpandedIds] = useState<Record<string, boolean>>({})
  const [createDialogOpen, setCreateDialogOpen] = useState(false)

  const { data: allMissions, isLoading, isError, error } = useMissions(projectId)
  const { data: allKRs } = useProjectKRs(projectId)
  const { data: availableSpaces = [] } = useProjectSpaces(projectId, { includeDeleted: true })
  const { isPinned, togglePin } = usePinnedMissions(projectId)

  const spacesWithKRs = useMemo(() => {
    if (!allKRs || !availableSpaces.length) return []
    const withKRs = new Set<string>()
    for (const [, kr] of allKRs) {
      if (kr.spaceId) withKRs.add(kr.spaceId)
    }
    return availableSpaces.filter(space => withKRs.has(space.spaceId))
  }, [allKRs, availableSpaces])

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

  const visibleMissions = useMemo(() => {
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

    if (spaceFilter) {
      const missionsWithSpace = new Set<string>()
      if (allKRs) {
        for (const [, kr] of allKRs) {
          if (kr.spaceId === spaceFilter) missionsWithSpace.add(kr.missionId)
        }
      }
      list = list.filter(mission => missionsWithSpace.has(mission.id))
    }

    return [...list].sort((a, b) => {
      const ap = isPinned(a.id) ? 0 : 1
      const bp = isPinned(b.id) ? 0 : 1
      return ap - bp
    })
  }, [allMissions, statusFilter, searchQuery, spaceFilter, allKRs, isPinned])

  const allExpanded = visibleMissions.length > 0 && visibleMissions.every(mission => !!expandedIds[mission.id])

  function toggleAll() {
    const nextExpanded = !allExpanded
    setExpandedIds(prev => {
      const next = { ...prev }
      for (const mission of visibleMissions) next[mission.id] = nextExpanded
      return next
    })
  }

  const selectedSpace = spaceFilter ? spacesWithKRs.find(space => space.spaceId === spaceFilter) : undefined
  const selectedSpaceLabel = spaceFilter ? (selectedSpace ? spaceSummaryLabel(selectedSpace) : spaceFilter) : null

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
              style={{ fontSize: embedded ? '19px' : '28px', fontWeight: 700, letterSpacing: embedded ? '-0.36px' : '-0.56px', lineHeight: embedded ? 1.18 : 1.14 }}
            >
              Missions
            </h1>
            <p className="m-0 mt-1 text-[var(--text-3)]" style={{ fontSize: embedded ? '12px' : '13px', letterSpacing: embedded ? '-0.12px' : '-0.08px', lineHeight: 1.45 }}>
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
          <div className="flex items-center gap-0.5 overflow-x-auto flex-1">
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
                    fontSize: embedded ? '12px' : '13px',
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
                        fontSize: '11px',
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
                style={{ fontSize: '12px', letterSpacing: '-0.08px' }}
              />
            </div>

            {spacesWithKRs.length > 0 && (
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <button
                    className={cn(
                      'inline-flex items-center gap-1.5 px-2.5 py-1.5 rounded-[var(--r-md)] border cursor-pointer transition-colors',
                      'border-[color-mix(in_srgb,var(--border)_54%,transparent)] bg-[var(--dashboard-subsurface-bg)]',
                      spaceFilter ? 'text-[var(--text-1)]' : 'text-[var(--text-3)] hover:text-[var(--text-2)]',
                    )}
                    style={{ fontSize: '12px', letterSpacing: '-0.08px' }}
                  >
                    <Users size={11} />
                    {selectedSpaceLabel ?? 'Space'}
                    <ChevronDown size={10} />
                  </button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="start" className="min-w-[140px]">
                  <DropdownMenuItem
                    className={cn('text-[12px]', !spaceFilter && 'text-[var(--accent)] font-medium')}
                    onClick={() => setSpaceFilter('')}
                  >
                    All spaces
                  </DropdownMenuItem>
                  {spacesWithKRs.map(space => (
                    <DropdownMenuItem
                      key={space.spaceId}
                      className={cn('text-[12px]', spaceFilter === space.spaceId && 'text-[var(--accent)] font-medium')}
                      onClick={() => setSpaceFilter(space.spaceId)}
                    >
                      {spaceSummaryLabel(space)}
                    </DropdownMenuItem>
                  ))}
                </DropdownMenuContent>
              </DropdownMenu>
            )}

            {visibleMissions.length > 0 && (
              <button
                onClick={toggleAll}
                className="ml-auto inline-flex items-center gap-1 text-[var(--text-3)] hover:text-[var(--text-2)] transition-colors bg-transparent border-none cursor-pointer p-0 shrink-0"
                style={{ fontSize: '11px', letterSpacing: '-0.06px' }}
              >
                <ChevronsUpDown size={11} />
                {allExpanded ? 'Collapse all' : 'Expand all'}
              </button>
            )}
          </div>
        )}
      </div>

      <div className={cn('flex-1 min-h-0 overflow-y-auto', embedded ? 'px-[var(--dashboard-context-gutter)] py-4' : 'px-6 py-2 max-w-4xl mx-auto w-full')}>
        {isLoading && <MissionsSkeleton embedded={embedded} />}

        {isError && (
          <div className="flex items-center gap-2 px-4 py-3 rounded-[8px] bg-[var(--dashboard-subsurface-bg)] text-[13px] text-[var(--red)]" style={{ letterSpacing: '-0.08px' }}>
            <AlertCircle size={16} />
            <span>Failed to load missions: {error instanceof Error ? error.message : 'Unknown error'}</span>
          </div>
        )}

        {!isLoading && !isError && visibleMissions.length === 0 && (
          <div className="flex flex-col items-center justify-center py-16 text-center">
            <Target size={36} className="text-[var(--text-3)] opacity-25 mb-4" />
            {allMissions && allMissions.length === 0 ? (
              <>
                <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '17px', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
                  No missions yet
                </h3>
                <p className="text-[var(--text-3)] mb-5 max-w-sm" style={{ fontSize: '14px', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
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
                <h3 className="text-[var(--text-1)] mb-1.5" style={{ fontSize: '17px', fontWeight: 600, letterSpacing: '-0.24px', lineHeight: 1.24 }}>
                  No results
                </h3>
                <p className="text-[var(--text-3)] mb-5" style={{ fontSize: '14px', letterSpacing: '-0.224px', lineHeight: 1.47 }}>
                  Try a different filter or search term.
                </p>
                <Button variant="secondary" onClick={() => { setStatusFilter('all'); setSearchQuery(''); setSpaceFilter('') }} style={{ letterSpacing: '-0.12px' }}>
                  Clear filters
                </Button>
              </>
            )}
          </div>
        )}

        {!isLoading && !isError && visibleMissions.length > 0 && (
          <div className="flex flex-col gap-0.5">
            {visibleMissions.map((mission) => (
              <MissionEditor
                key={mission.id}
                mission={mission}
                expanded={!!expandedIds[mission.id]}
                onExpandedChange={(value) => setExpandedIds(prev => ({ ...prev, [mission.id]: value }))}
                isPinned={isPinned(mission.id)}
                onTogglePin={() => togglePin(mission.id)}
              />
            ))}
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
