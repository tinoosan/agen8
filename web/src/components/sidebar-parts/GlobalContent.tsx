/**
 * GlobalSidebarContent — shown when no project is selected.
 * Displays aggregated spaces and missions across all projects,
 * each tagged with the project they belong to. Clicking a row
 * navigates into that project + space/mission.
 */
import { useMemo } from 'react'
import { useLocation } from 'wouter'
import { useQuery } from '@tanstack/react-query'
import { Clock, CircleCheck, CircleAlert, Target } from 'lucide-react'
import { rpcCall } from '../../lib/rpc'
import { spaceLink, missionDetailLink } from '../../lib/routing'
import { projectDisplayName } from '../../lib/spaceHelpers'
import { sanitizeSpaceTitle } from '../../lib/displaySanitizers'
import { useProjects } from '../../hooks/useProjects'
import { resolveSpaceIcon, spaceColorVar } from '../../lib/spaceCustomization'
import type { Space, MissionView } from '../../lib/types'
import { Skeleton } from '@/components/ui/skeleton'

/* ── Mission status icon (mirrors MissionSection) ────── */

function MissionStatusIcon({ status }: { status: MissionView['status'] }) {
  switch (status) {
    case 'active':
      return <Clock size={12} className="shrink-0 text-[var(--accent)]" />
    case 'completed':
      return <CircleCheck size={12} className="shrink-0 text-[var(--green)]" />
    case 'paused':
      return <CircleAlert size={12} className="shrink-0 text-[var(--amber)]" />
    default:
      return <Target size={12} className="shrink-0 text-[var(--text-3)]" />
  }
}

/* ── Section label (matches Sidebar's SidebarSectionLabel) ── */

function SectionLabel({ children, count }: { children: React.ReactNode; count?: number }) {
  return (
    <div className="mx-3.5 mb-1 mt-2.5 flex items-center gap-2">
      <span className="flex-1 text-[10px] font-semibold uppercase text-[var(--text-3)]" style={{ letterSpacing: '0.06em' }}>
        {children}
      </span>
      {count != null && count > 0 && (
        <span className="text-[10px] tabular-nums text-[var(--text-4)]">{count}</span>
      )}
    </div>
  )
}

/* ── Loading skeleton ──────────────────────────────────── */

function SectionSkeleton({ rows }: { rows: number }) {
  return (
    <div className="px-4 py-2 flex flex-col gap-1">
      {Array.from({ length: rows }, (_, i) => (
        <Skeleton key={i} className="h-[22px] rounded" />
      ))}
    </div>
  )
}

/* ── Main component ────────────────────────────────────── */

export function GlobalSidebarContent() {
  const [, navigate] = useLocation()
  const projectsQuery = useProjects()
  const projects = useMemo(() => projectsQuery.data ?? [], [projectsQuery.data])

  const projectIds = useMemo(
    () => projects.filter(p => p.status === 'open').map(p => p.id),
    [projects],
  )

  // Build project name lookup
  const projectNameMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const p of projects) {
      map.set(p.id, projectDisplayName(p))
    }
    return map
  }, [projects])

  // Fetch all spaces across projects
  const allSpacesQuery = useQuery<Array<Space & { _projectId: string }>>({
    queryKey: ['sidebar.globalSpaces', projectIds],
    queryFn: async () => {
      const results = await Promise.all(
        projectIds.map(async (pid) => {
          const res = await rpcCall<{ spaces: Space[] }>('space.list', { projectId: pid, status: 'open' })
          return (res.spaces ?? []).map(s => ({ ...s, _projectId: pid }))
        }),
      )
      return results.flat()
    },
    enabled: projectIds.length > 0,
    staleTime: 10_000,
    refetchInterval: 10_000,
    retry: false,
  })

  // Fetch all missions across projects
  const allMissionsQuery = useQuery<Array<MissionView & { _projectId: string }>>({
    queryKey: ['sidebar.globalMissions', projectIds],
    queryFn: async () => {
      const results = await Promise.all(
        projectIds.map(async (pid) => {
          const res = await rpcCall<{ missions: MissionView[] }>('mission.list', { projectId: pid })
          return (res.missions ?? []).map(m => ({ ...m, _projectId: pid }))
        }),
      )
      return results.flat()
    },
    enabled: projectIds.length > 0,
    staleTime: 10_000,
    refetchInterval: 10_000,
    retry: false,
  })

  // Sort spaces by most recently updated
  const spaces = useMemo(() => {
    const list = allSpacesQuery.data ?? []
    return [...list].sort((a, b) => (b.updatedAt ?? '').localeCompare(a.updatedAt ?? ''))
  }, [allSpacesQuery.data])

  // Filter + sort missions: active/paused first, then by status priority
  const missions = useMemo(() => {
    const list = allMissionsQuery.data ?? []
    const statusOrder: Record<string, number> = { active: 0, paused: 1, completed: 2 }
    return list
      .filter(m => m.status === 'active' || m.status === 'paused' || m.status === 'completed')
      .sort((a, b) => (statusOrder[a.status] ?? 9) - (statusOrder[b.status] ?? 9))
      .slice(0, 10)
  }, [allMissionsQuery.data])

  const showMultipleProjects = projects.length > 1

  return (
    <>
      {/* Spaces section */}
      <SectionLabel count={spaces.length}>Spaces</SectionLabel>
      {allSpacesQuery.isLoading ? (
        <SectionSkeleton rows={3} />
      ) : spaces.length === 0 ? (
        <div className="px-4 py-2 text-[11px] text-[var(--text-4)]">
          No spaces yet
        </div>
      ) : (
        <div className="flex flex-col gap-0 px-1.5">
          {spaces.map(space => {
            const rawTitle = (space.title ?? '').trim()
            const name = sanitizeSpaceTitle(rawTitle) || 'Untitled space'
            const customization = space.customization ?? null
            const SpaceIcon = resolveSpaceIcon(customization?.icon)
            const accentVar = spaceColorVar(customization?.color)
            const projName = projectNameMap.get(space._projectId) ?? ''

            return (
              <button
                key={`${space._projectId}:${space.id}`}
                type="button"
                className="flex items-center gap-2 w-full mx-1 rounded-[6px] px-2.5 py-[5px] text-[13px] text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)] cursor-pointer border-0 bg-transparent transition-colors text-left"
                style={{ letterSpacing: '-0.08px' }}
                onClick={() => navigate(spaceLink(space._projectId, space.id))}
              >
                <SpaceIcon
                  size={13}
                  className="shrink-0"
                  style={{ color: accentVar ?? 'var(--text-3)' }}
                />
                <span className="flex-1 min-w-0 truncate">{name}</span>
                {showMultipleProjects && (
                  <span className="shrink-0 text-[10px] text-[var(--text-4)] tabular-nums ml-auto">
                    {projName}
                  </span>
                )}
              </button>
            )
          })}
        </div>
      )}

      <div className="h-px mx-3.5 my-1 bg-[var(--border)]" />

      {/* Missions section */}
      <SectionLabel count={missions.length}>Missions</SectionLabel>
      {allMissionsQuery.isLoading ? (
        <SectionSkeleton rows={2} />
      ) : missions.length === 0 ? (
        <div className="px-4 py-2 text-[11px] text-[var(--text-4)]">
          No active missions
        </div>
      ) : (
        <div className="flex flex-col gap-0">
          {missions.map(mission => {
            const projName = projectNameMap.get(mission._projectId) ?? ''

            return (
              <button
                key={`${mission._projectId}:${mission.id}`}
                type="button"
                className="flex items-center gap-2 px-3.5 py-[5px] text-left cursor-pointer border-0 bg-transparent hover:bg-[var(--bg-hover)] transition-colors w-full"
                onClick={() => navigate(missionDetailLink(mission._projectId, mission.id))}
              >
                <MissionStatusIcon status={mission.status} />
                <span className="flex-1 min-w-0 truncate text-[12px] text-[var(--text-2)] leading-tight">
                  {mission.title}
                </span>
                {showMultipleProjects && (
                  <span className="shrink-0 text-[10px] text-[var(--text-4)] tabular-nums ml-auto">
                    {projName}
                  </span>
                )}
              </button>
            )
          })}
        </div>
      )}
    </>
  )
}
