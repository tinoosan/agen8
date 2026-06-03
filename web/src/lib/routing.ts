import { useLocation } from 'wouter'
import { useProjects } from '../hooks/useProjects'
import { useStore, type DefaultProjectView } from './store'
import type { Project } from './types'

export type ActiveView = 'project' | 'board' | 'dashboard' | 'decisions' | 'missions' | 'actions' | 'strategy' | 'space'
export type DashboardPanel = 'overview' | 'missions' | 'actions' | 'decisions'

/* ── Route constants ──────────────────────────────── */

export const ROUTES = {
  HOME: '/',
  PROJECT: '/project/:projectId',
  BOARD: '/project/:projectId/board',
  DASHBOARD: '/project/:projectId/dashboard',
  SPACE: '/project/:projectId/space/:spaceId',
  MISSIONS: '/project/:projectId/missions',
  MISSION_DETAIL: '/project/:projectId/missions/:missionId',
  ACTIONS: '/project/:projectId/actions',
  ACTION_DETAIL: '/project/:projectId/actions/:actionId',
  STRATEGY_MAP: '/project/:projectId/strategy',
} as const

/* ── Link helpers for cross-surface navigation (F26) ── */

export function spaceLink(projectId: string, spaceId: string): string {
  return `/project/${encodeURIComponent(projectId)}/space/${encodeURIComponent(spaceId)}`
}

// Build a link to the board with a specific task pre-selected
export function boardTaskLink(projectId: string, spaceId: string, taskId: string): string {
  const base = `/project/${encodeURIComponent(projectId)}/space/${encodeURIComponent(spaceId)}`
  const params = new URLSearchParams({ tab: 'board' })
  if (taskId.trim()) params.set('task', taskId)
  return `${base}?${params.toString()}`
}

export function calendarLink(projectId: string, taskId?: string, _date?: string): string {
  const base = `/project/${encodeURIComponent(projectId)}/dashboard`
  if (!taskId) return base
  const params = new URLSearchParams({ task: taskId })
  return `${base}?${params.toString()}`
}

// Build a link to a mission detail view
export function missionDetailLink(projectId: string, missionId: string): string {
  return `/project/${encodeURIComponent(projectId)}/missions/${encodeURIComponent(missionId)}`
}

// Build a link to an action detail view
export function actionDetailLink(projectId: string, actionId: string): string {
  return `/project/${encodeURIComponent(projectId)}/actions/${encodeURIComponent(actionId)}`
}

export function dashboardLink(projectId: string, params?: { panel?: DashboardPanel; type?: 'all' | 'oa' | 'escalation' }): string {
  const base = `/project/${encodeURIComponent(projectId)}/dashboard`
  const search = new URLSearchParams()
  if (params?.panel && params.panel !== 'overview') search.set('panel', params.panel)
  if (params?.panel === 'actions' && params.type && params.type !== 'all') search.set('type', params.type)
  const qs = search.toString()
  return qs ? `${base}?${qs}` : base
}

export function missionsPanelLink(projectId: string): string {
  return dashboardLink(projectId, { panel: 'missions' })
}

export function actionsPanelLink(projectId: string, type?: 'all' | 'oa' | 'escalation'): string {
  return dashboardLink(projectId, { panel: 'actions', type: type ?? 'all' })
}

export function decisionsPanelLink(projectId: string): string {
  return dashboardLink(projectId, { panel: 'decisions' })
}

export function decisionsLink(projectId: string): string {
  return `/project/${encodeURIComponent(projectId)}/decisions`
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
  /** spaceId from a /space/:spaceId URL segment. */
  urlSpaceId: string | null
  activeView: ActiveView
}

function parseLocation(pathname: string): ParsedRoute {
  const segments = pathname.split('/').filter(Boolean)

  if (segments[0] !== 'project' || !segments[1]) {
    return { projectId: null, urlSpaceId: null, activeView: 'project' }
  }

  const projectId = decodeURIComponent(segments[1])
  let urlSpaceId: string | null = null
  let activeView: ActiveView = 'project'

  if (segments[2] === 'space' && segments[3]) {
    urlSpaceId = decodeURIComponent(segments[3])
    activeView = 'space'
  } else if (segments[2] === 'board') {
    activeView = 'board'
  } else if (segments[2] === 'dashboard') {
    activeView = 'dashboard'
  } else if (segments[2] === 'missions') {
    activeView = 'missions'
  } else if (segments[2] === 'decisions') {
    activeView = 'decisions'
  } else if (segments[2] === 'actions') {
    activeView = 'actions'
  } else if (segments[2] === 'strategy') {
    activeView = 'strategy'
  }

  return { projectId, urlSpaceId, activeView }
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
  /**
   * The stable space ID for the currently focused workspace.
   * On /space/ routes: derived directly from the URL (source of truth).
   * On project-level routes: derived from the Zustand store via route normalization.
   * Null until a space route is loaded or normalization resolves it.
   */
  focusedSpaceId: string | null
  /** Navigate to /project/:projectId/space/:spaceId */
  setFocusedSpaceId: (id: string | null) => void
  activeView: ActiveView
  projectLoading: boolean

  setFocusedProjectRoot: (root: string | null) => void
  setActiveView: (view: ActiveView) => void
}

export function useNavigation(): NavigationState {
  const [location, navigate] = useLocation()
  const projectsQuery = useProjects()
  const projects = projectsQuery.data ?? []
  const setStoreSpaceId = useStore((s) => s.setFocusedSpaceId)
  const defaultProjectView = useStore((s) => s.defaultProjectView)

  const { projectId, urlSpaceId, activeView } = parseLocation(location)

  const project = findProjectById(projects, projectId)
  const focusedProjectRoot = project?.root ?? null
  const projectLoading = projectId !== null && projectsQuery.isLoading

  // URL is the source of truth on /space/ routes.
  // Fall back to store for pages that don't have a space in the URL.
  const storeSpaceId = useStore((s) => s.focusedSpaceId)
  const focusedSpaceId = urlSpaceId ?? storeSpaceId

  return {
    projectId,
    focusedProjectRoot,
    focusedSpaceId,
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

    setFocusedSpaceId: (id: string | null) => {
      if (!id || !projectId) {
        setStoreSpaceId(null)
        if (projectId) navigate(`/project/${encodeURIComponent(projectId)}`)
        else navigate('/')
        return
      }
      const spaceId = id.trim()
      if (!spaceId.startsWith('space-')) {
        setStoreSpaceId(null)
        navigate(`/project/${encodeURIComponent(projectId)}`)
        return
      }
      setStoreSpaceId(spaceId)
      navigate(`/project/${encodeURIComponent(projectId)}/space/${encodeURIComponent(spaceId)}`)
    },

    setActiveView: (view: ActiveView) => {
      if (view === 'project' || !projectId) { navigate('/'); return }
      if (view === 'dashboard') { navigate(`/project/${encodeURIComponent(projectId)}`); return }
      navigate(`/project/${encodeURIComponent(projectId)}/${view}`)
    },

  }
}
