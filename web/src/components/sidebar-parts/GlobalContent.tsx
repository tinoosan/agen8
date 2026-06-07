/**
 * GlobalSidebarContent — shown when no project is selected.
 * Displays projects and missions across projects without the old space layer.
 */
import { useMemo } from 'react'
import { useLocation } from 'wouter'
import { useQuery } from '@tanstack/react-query'
import { Clock, CircleCheck, CircleAlert, Target, FolderOpen } from 'lucide-react'
import { rpcUnwrapList } from '../../lib/rpc'
import { qk } from '../../lib/queryKeys'
import { dashboardLink, missionDetailLink } from '../../lib/routing'
import { useProjects } from '../../hooks/useProjects'
import type { MissionView } from '../../lib/types'
import { projectDisplayName } from '../../lib/projectHelpers'
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
      <span className="flex-1 text-[0.625rem] font-semibold uppercase text-[var(--text-3)]" style={{ letterSpacing: '0.06em' }}>
        {children}
      </span>
      {count != null && count > 0 && (
        <span className="text-[0.625rem] tabular-nums text-[var(--text-4)]">{count}</span>
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

  const projectNameMap = useMemo(() => {
    const map = new Map<string, string>()
    for (const p of projects) {
      map.set(p.id, projectDisplayName(p))
    }
    return map
  }, [projects])

  const allMissionsQuery = useQuery<Array<MissionView & { _projectId: string }>>({
    queryKey: qk.sidebarGlobalMissions(projectIds),
    queryFn: async () => {
      const results = await Promise.all(
        projectIds.map(async (pid) => {
          const missions = await rpcUnwrapList<MissionView>('mission.list', { projectId: pid }, 'missions')
          return missions.map(m => ({ ...m, _projectId: pid }))
        }),
      )
      return results.flat()
    },
    enabled: projectIds.length > 0,
    staleTime: 10_000,
    refetchInterval: 10_000,
    retry: false,
  })

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
      <SectionLabel count={projects.length}>Projects</SectionLabel>
      {projectsQuery.isLoading ? (
        <SectionSkeleton rows={3} />
      ) : projects.length === 0 ? (
        <div className="px-4 py-2 text-[0.6875rem] text-[var(--text-4)]">
          No projects yet
        </div>
      ) : (
        <div className="flex flex-col gap-0 px-1.5">
          {projects.map(project => (
            <button
              key={project.id}
              type="button"
              className="flex items-center gap-2 w-full mx-1 rounded-[6px] px-2.5 py-[5px] text-[0.8125rem] text-[var(--text-3)] hover:bg-[var(--bg-hover)] hover:text-[var(--text-2)] cursor-pointer border-0 bg-transparent transition-colors text-left"
              style={{ letterSpacing: '-0.08px' }}
              onClick={() => navigate(dashboardLink(project.id))}
            >
              <FolderOpen size={13} className="shrink-0 text-[var(--text-3)]" />
              <span className="flex-1 min-w-0 truncate">{projectDisplayName(project)}</span>
            </button>
          ))}
        </div>
      )}

      <div className="h-px mx-3.5 my-1 bg-[var(--border)]" />

      {/* Missions section */}
      <SectionLabel count={missions.length}>Missions</SectionLabel>
      {allMissionsQuery.isLoading ? (
        <SectionSkeleton rows={2} />
      ) : missions.length === 0 ? (
        <div className="px-4 py-2 text-[0.6875rem] text-[var(--text-4)]">
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
                <span className="flex-1 min-w-0 truncate text-[0.75rem] text-[var(--text-2)] leading-tight">
                  {mission.title}
                </span>
                {showMultipleProjects && (
                  <span className="shrink-0 text-[0.625rem] text-[var(--text-4)] tabular-nums ml-auto">
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
