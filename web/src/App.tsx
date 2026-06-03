import React, { Suspense, useEffect, useRef } from 'react'
import { Redirect, Route, Switch, useLocation, useSearch } from 'wouter'
import { useStore } from './lib/store'
import { actionsPanelLink, missionsPanelLink, useNavigation } from './lib/routing'
import { useAuth } from './hooks/useAuth'
import { lazyWithRetry } from './lib/lazyWithRetry'
import Sidebar from './components/Sidebar'
import { Toaster } from './components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarProvider } from '@/components/ui/sidebar'
import { PageErrorBoundary } from './components/ErrorBoundary'

const Project = lazyWithRetry(() => import('./pages/Project'), 'pages/Project')
const Login = lazyWithRetry(() => import('./pages/Login'), 'pages/Login')
const Account = lazyWithRetry(() => import('./pages/Account'), 'pages/Account')
const Dashboard = lazyWithRetry(() => import('./pages/Dashboard'), 'pages/Dashboard')
const Board = lazyWithRetry(() => import('./pages/Board'), 'pages/Board')
const SpaceFocus = lazyWithRetry(() => import('./pages/SpaceFocus'), 'pages/SpaceFocus')
const Notifications = lazyWithRetry(() => import('./pages/Notifications'), 'pages/Notifications')
const Locations = lazyWithRetry(() => import('./pages/Locations'), 'pages/Locations')
const MissionDetail = lazyWithRetry(() => import('./pages/MissionDetail'), 'pages/MissionDetail')
const StrategyMap = lazyWithRetry(() => import('./pages/StrategyMap'), 'pages/StrategyMap')
const Decisions = lazyWithRetry(() => import('./pages/Decisions'), 'pages/Decisions')
const ActionDetail = lazyWithRetry(() => import('./pages/ActionDetail'), 'pages/ActionDetail')

const CommandPalette = lazyWithRetry(() => import('./components/CommandPalette'), 'components/CommandPalette')

function SpaceRoute({ params }: { params: { spaceId: string } }) {
  return <SpaceFocus spaceId={params.spaceId} />
}

function ActionDetailRoute({ params }: { params: { projectId: string; actionId: string } }) {
  return <ActionDetail params={params} />
}

function MissionsRouteRedirect({ params }: { params: { projectId: string } }) {
  return <Redirect to={missionsPanelLink(params.projectId)} />
}

function ActionsRouteRedirect({ params }: { params: { projectId: string } }) {
  const search = useSearch()
  const type = new URLSearchParams(search).get('type')
  const filter = type === 'oa' || type === 'escalation' ? type : 'all'
  return <Redirect to={actionsPanelLink(params.projectId, filter)} />
}

function HeartbeatsRouteRedirect({ params }: { params: { projectId: string } }) {
  const focusedSpaceId = useStore((s) => s.focusedSpaceId)
  if (!focusedSpaceId) return <Redirect to={`/project/${encodeURIComponent(params.projectId)}/dashboard`} />

  return (
    <Redirect
      to={`/project/${encodeURIComponent(params.projectId)}/space/${encodeURIComponent(focusedSpaceId)}`}
    />
  )
}


const Spinner = () => (
  <div className="flex items-center justify-center h-full">
    <span className="spinner spinner-md" />
  </div>
)

/** Full-page loading shell — shows a skeleton sidebar + content area so the
 *  transition into the real app feels seamless rather than a blank void. */
const AppShell = () => (
  <div className="flex h-dvh animate-fade-in">
    {/* Sidebar skeleton */}
    <div className="w-[260px] shrink-0 bg-[var(--bg-surface)] flex flex-col gap-4 p-4">
      <div className="h-5 w-20 rounded bg-[var(--bg-elevated)] skeleton" />
      <div className="flex flex-col gap-2 mt-4">
        <div className="h-4 w-full rounded bg-[var(--bg-elevated)] skeleton" />
        <div className="h-4 w-3/4 rounded bg-[var(--bg-elevated)] skeleton" />
        <div className="h-4 w-5/6 rounded bg-[var(--bg-elevated)] skeleton" />
      </div>
      <div className="flex flex-col gap-2 mt-6">
        <div className="h-3 w-16 rounded bg-[var(--bg-elevated)] skeleton" />
        <div className="h-4 w-full rounded bg-[var(--bg-elevated)] skeleton" />
        <div className="h-4 w-2/3 rounded bg-[var(--bg-elevated)] skeleton" />
      </div>
    </div>
    {/* Content area skeleton */}
    <div className="flex-1 flex flex-col items-center justify-center">
      <span className="spinner spinner-md" />
    </div>
  </div>
)

export default function App() {
  const { paletteOpen, theme, resetEphemeral, setFocusedSpaceId } = useStore()
  const { focusedSpaceId } = useNavigation()
  const auth = useAuth()
  const [location, navigate] = useLocation()
  const isAuthRoute = location === '/login'
  const isSpaceRoute = /^\/project\/[^/]+\/space\//.test(location)

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
  }, [theme])

  useEffect(() => {
    if (isSpaceRoute && focusedSpaceId) {
      setFocusedSpaceId(focusedSpaceId)
    }
  }, [focusedSpaceId, isSpaceRoute, setFocusedSpaceId])

  useEffect(() => {
    function onKey(e: KeyboardEvent) {
      const isMod = e.metaKey || e.ctrlKey
      const key = String(e.key ?? '').toLowerCase()
      if (isMod && !e.shiftKey && key === 'k') {
        e.preventDefault()
        useStore.getState().setPaletteOpen(true)
      }
      if (isMod && e.shiftKey && key === 'p') {
        e.preventDefault()
        useStore.getState().setPaletteOpen(true)
      }
      if (e.key === 'Escape') {
        useStore.getState().setPaletteOpen(false)
      }
    }
    window.addEventListener('keydown', onKey)
    return () => window.removeEventListener('keydown', onKey)
  }, [])

  /* Reset ephemeral UI (panels, pickers) when the focused workspace changes */
  const prevFocusKey = useRef(`${focusedSpaceId}`)
  useEffect(() => {
    const key = `${focusedSpaceId}`
    if (key !== prevFocusKey.current) {
      prevFocusKey.current = key
      resetEphemeral()
    }
  }, [focusedSpaceId, resetEphemeral])

  useEffect(() => {
    if (auth.isLoading) return
    if (!auth.isAuthenticated && !isAuthRoute) {
      navigate('/login')
      return
    }
    if (auth.isAuthenticated && isAuthRoute) {
      navigate('/')
    }
  }, [auth.isAuthenticated, auth.isLoading, isAuthRoute, navigate])

  if (isAuthRoute) {
    if (!auth.isAuthenticated) {
      return (
        <Suspense fallback={<Spinner />}>
          <Switch>
            <Route path="/login" component={Login} />
          </Switch>
        </Suspense>
      )
    }
    return <AppShell />
  }

  if (auth.isLoading) {
    return <AppShell />
  }

  if (!auth.isAuthenticated) {
    return <AppShell />
  }

  return (
    <TooltipProvider>
      <SidebarProvider
        defaultOpen={true}
        className="h-dvh flex-col"
        style={{ '--sidebar-width': '260px', '--sidebar-width-icon': '56px' } as React.CSSProperties}
      >
        <div className="flex flex-1 min-h-0 w-full">
          <a
            href="#main-content"
            className="skip-to-content"
          >
            Skip to content
          </a>
          <Sidebar />
          <main id="main-content" className="app-main">
            <Suspense fallback={<Spinner />}>
              <PageErrorBoundary>
                <Switch>
                  <Route path="/project/:projectId/space/:spaceId" component={SpaceRoute} />
                  <Route path="/project/:projectId/messages">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId/missions/:missionId" component={MissionDetail} />
                  <Route path="/project/:projectId/missions" component={MissionsRouteRedirect} />
                  <Route path="/project/:projectId/strategy">{(params) => <StrategyMap projectId={params.projectId} />}</Route>
                  <Route path="/project/:projectId/actions/:actionId" component={ActionDetailRoute} />
                  <Route path="/project/:projectId/actions" component={ActionsRouteRedirect} />
                  <Route path="/project/:projectId/decisions" component={Decisions} />
                  <Route path="/project/:projectId/heartbeats" component={HeartbeatsRouteRedirect} />
                  <Route path="/project/:projectId/builder">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId/roles">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId/board" component={Board} />
                  <Route path="/project/:projectId/dashboard" component={Dashboard} />
                  <Route path="/project/:projectId/metrics">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId/notifications" component={Notifications} />
                  <Route path="/project/:projectId" component={Dashboard} />
                  <Route path="/account" component={Account} />
                  <Route path="/login" component={Login} />
                  <Route path="/locations" component={Locations} />
                  <Route path="/" component={Project} />
                </Switch>
              </PageErrorBoundary>
            </Suspense>
          </main>
        </div>
        <Suspense fallback={null}>
          {paletteOpen && <CommandPalette />}
        </Suspense>
        <Toaster />
      </SidebarProvider>
    </TooltipProvider>
  )
}
