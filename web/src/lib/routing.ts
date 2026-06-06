import { useLocation } from 'wouter'
import { useProjects } from '../hooks/useProjects'
import { useStore, type DefaultProjectView } from './store'
import type { Project } from './types'

export type ActiveView = 'project' | 'dashboard' | 'decisions' | 'missions' | 'strategy' | 'members'
export type DashboardPanel = 'missions' | 'decisions' | 'tasks'

/* ── Route constants ──────────────────────────────── */

export const ROUTES = {
  HOME: '/',
  PROJECT: '/project/:projectId',
  DASHBOARD: '/project/:projectId/dashboard',
  MISSIONS: '/project/:projectId/missions',
  MISSION_DETAIL: '/project/:projectId/missions/:missionId',
  TASK_DETAIL: '/project/:projectId/tasks/:taskId',
  DECISION_DETAIL: '/project/:projectId/decisions/:decisionId',
  STRATEGY_MAP: '/project/:projectId/strategy',
  MEMBERS: '/project/:projectId/members',
} as const

/* ── Link helpers for cross-surface navigation (F26) ── */

export function boardTaskLink(projectId: string, taskId: string): string {
  const base = dashboardLink(projectId)
  const params = new URLSearchParams()
  params.set('panel', 'tasks')
  if (taskId.trim()) params.set('task', taskId)
  const qs = params.toString()
  return qs ? `${base}?${qs}` : base
}

export function calendarLink(projectId: string, taskId?: string): string {
  const base = `/project/${encodeURIComponent(projectId)}/dashboard`
  if (!taskId) return base
  const params = new URLSearchParams({ task: taskId })
  return `${base}?${params.toString()}`
}

// Build a link to a mission detail view
export function missionDetailLink(projectId: string, missionId: string): string {
  return `/project/${encodeURIComponent(projectId)}/missions/${encodeURIComponent(missionId)}`
}

// Build a link to a task detail view
export function taskDetailLink(projectId: string, taskId: string): string {
  return `/project/${encodeURIComponent(projectId)}/tasks/${encodeURIComponent(taskId)}`
}

// Build a link to a decision detail view
export function decisionDetailLink(projectId: string, decisionId: string): string {
  return `/project/${encodeURIComponent(projectId)}/decisions/${encodeURIComponent(decisionId)}`
}

export function dashboardLink(projectId: string, params?: { panel?: DashboardPanel; status?: string }): string {
  const base = `/project/${encodeURIComponent(projectId)}/dashboard`
  const search = new URLSearchParams()
  if (params?.panel) search.set('panel', params.panel)
  // 'all' is the default view, so omit it to keep links clean and deep-linkable.
  if (params?.status && params.status !== 'all') search.set('status', params.status)
  const qs = search.toString()
  return qs ? `${base}?${qs}` : base
}

export function missionsPanelLink(projectId: string): string {
  return dashboardLink(projectId, { panel: 'missions' })
}

export function decisionsPanelLink(projectId: string): string {
  return dashboardLink(projectId, { panel: 'decisions' })
}

export function tasksPanelLink(projectId: string): string {
  return dashboardLink(projectId, { panel: 'tasks' })
}

// Open the Tasks panel pre-filtered to a status bucket (e.g. from a dashboard tile).
export function filteredTasksLink(projectId: string, status: string): string {
  return dashboardLink(projectId, { panel: 'tasks', status })
}

export function decisionsLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/decisions`
}

export function membersLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/members`
}

// Build a link to the strategy map, optionally focusing a specific node
export function strategyMapLink(projectId: string, focusNodeId?: string): string {
  const base = `/project/${encodeURIComponent(projectId)}/strategy`
  return focusNodeId ? `${base}?focus=${encodeURIComponent(focusNodeId)}` : base
}

export function projectDefaultViewLink(projectId: string, view: DefaultProjectView): string {
  switch (view) {
    case 'strategy':
      return strategyMapLink(projectId)
    case 'dashboard':
      return dashboardLink(projectId)
  }
}

// Build a link to the missions list
export function missionsLink(projectId: string): string {
  return missionsPanelLink(projectId)
}

/* ── URL parser ───────────────────────────────────── */

interface ParsedRoute {
  projectId: string | null
  activeView: ActiveView
}

function parseLocation(pathname: string): ParsedRoute {
  const segments = pathname.split('/').filter(Boolean)

  if (segments[0] !== 'project' || !segments[1]) {
    return { projectId: null, activeView: 'project' }
  }

  const projectId = decodeURIComponent(segments[1])
  let activeView: ActiveView = 'project'

  if (segments[2] === 'dashboard') {
    activeView = 'dashboard'
  } else if (segments[2] === 'tasks') {
    activeView = 'dashboard'
  } else if (segments[2] === 'missions') {
    activeView = 'missions'
  } else if (segments[2] === 'decisions') {
    activeView = 'decisions'
  } else if (segments[2] === 'strategy') {
    activeView = 'strategy'
  } else if (segments[2] === 'members') {
    activeView = 'members'
  }

  return { projectId, activeView }
}

/* ── Project lookup ───────────────────────────────── */

function findProjectById(
  projects: Project[],
  projectId: string | null,
): Project | null {
  if (!projectId || projects.length === 0) return null
  return projects.find(p => p.id === projectId) ?? null
}

function findProjectByRoot(
  projects: Project[],
  root: string,
): Project | null {
  return projects.find(p => p.root === root) ?? null
}

/* ── Main navigation hook ─────────────────────────── */

export interface NavigationState {
  projectId: string | null
  focusedProjectRoot: string | null
  activeView: ActiveView
  projectLoading: boolean

  setFocusedProjectRoot: (root: string | null) => void
  setActiveView: (view: ActiveView) => void
}

export function useNavigation(): NavigationState {
  const [location, navigate] = useLocation()
  const projectsQuery = useProjects()
  const projects = projectsQuery.data ?? []
  const defaultProjectView = useStore((s) => s.defaultProjectView)

  const { projectId, activeView } = parseLocation(location)

  const project = findProjectById(projects, projectId)
  const focusedProjectRoot = project?.root ?? null
  const projectLoading = projectId !== null && projectsQuery.isLoading

  return {
    projectId,
    focusedProjectRoot,
    activeView,
    projectLoading,

    setFocusedProjectRoot: (root: string | null) => {
      if (!root) { navigate('/'); return }
      const proj = findProjectByRoot(projects, root)
      if (proj?.id) {
        navigate(projectDefaultViewLink(proj.id, defaultProjectView))
      } else {
        navigate('/')
      }
    },

    setActiveView: (view: ActiveView) => {
      if (view === 'project' || !projectId) { navigate('/'); return }
      if (view === 'dashboard') { navigate(`/project/${encodeURIComponent(projectId)}`); return }
      navigate(`/project/${encodeURIComponent(projectId)}/${view}`)
    },

  }
}
