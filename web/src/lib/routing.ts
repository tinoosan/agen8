import { useLocation } from 'wouter'
import { useProjects } from '../hooks/useProjects'
import { useStore, type DefaultProjectView } from './store'
import type { Project } from './types'

export type ActiveView = 'project' | 'dashboard' | 'decisions' | 'missions' | 'strategy' | 'members' | 'pulse' | 'tasks'
// The dashboard context rail no longer carries tasks — they live on the Pulse
// page now (see tasksPanelLink). Only missions and decisions remain as rail tabs.
export type DashboardPanel = 'missions' | 'decisions'

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
  PULSE: '/project/:projectId/pulse',
} as const

/* ── Link helpers for cross-surface navigation (F26) ── */

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

// Tasks live on their own dedicated page (not the dashboard rail), so the "go
// to tasks" affordances resolve there.
export function tasksLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/tasks`
}

export function tasksPanelLink(projectId: string): string {
  return tasksLink(projectId)
}

// Open the Tasks list pre-filtered to a status bucket (e.g. from a dashboard
// tile). The Tasks page reads ?status= off the URL. 'all' is the default, so
// it's omitted.
export function filteredTasksLink(projectId: string, status: string): string {
  const base = tasksLink(projectId)
  if (!status || status === 'all') return base
  return `${base}?status=${encodeURIComponent(status)}`
}

export function decisionsLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/decisions`
}

export function membersLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/members`
}

export function pulseLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/pulse`
}

// Build a link to the strategy map, optionally focusing a specific node
export function strategyMapLink(projectId: string, focusNodeId?: string): string {
  const base = `/project/${encodeURIComponent(projectId)}/strategy`
  return focusNodeId ? `${base}?focus=${encodeURIComponent(focusNodeId)}` : base
}

export type MapNodeKind = 'mission' | 'keyResult' | 'task' | 'decision'

// Build the strategy-map node id for an entity. Missions and key results are
// keyed by their raw id in the map; tasks and decisions are prefixed. Keeping
// this mapping in one place means the detail-page "view in context map"
// affordances and the map's node-building code (useLeafNodes / useMissionKRNodes)
// can't quietly drift apart.
export function mapNodeId(kind: MapNodeKind, id: string): string {
  switch (kind) {
    case 'task':
      return `task:${id}`
    case 'decision':
      return `decision:${id}`
    case 'mission':
    case 'keyResult':
      return id
  }
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
    // Both the /tasks list and /tasks/:taskId detail highlight the Tasks nav.
    activeView = 'tasks'
  } else if (segments[2] === 'missions') {
    activeView = 'missions'
  } else if (segments[2] === 'decisions') {
    activeView = 'decisions'
  } else if (segments[2] === 'strategy') {
    activeView = 'strategy'
  } else if (segments[2] === 'members') {
    activeView = 'members'
  } else if (segments[2] === 'pulse' || segments[2] === 'activity' || segments[2] === 'metrics') {
    // Pulse merges the former Activity and Metrics pages. The legacy segments
    // still resolve to the pulse view so a deep link highlights the right nav
    // item during the brief moment before App redirects it to /pulse.
    activeView = 'pulse'
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
