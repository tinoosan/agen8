import React, { Suspense, useEffect, useRef } from 'react'
import { Redirect, Route, Switch, useLocation } from 'wouter'
import { Menu, Search } from 'lucide-react'
import { useStore } from './lib/store'
import { brandIconFor } from './lib/brandIcon'
import { missionsPanelLink, useNavigation, type ActiveView } from './lib/routing'
import { useAuth } from './hooks/useAuth'
import { lazyWithRetry } from './lib/lazyWithRetry'
import Sidebar from './components/Sidebar'
import { Toaster } from './components/ui/sonner'
import { TooltipProvider } from '@/components/ui/tooltip'
import { SidebarProvider, useSidebar } from '@/components/ui/sidebar'
import { PageErrorBoundary } from './components/ErrorBoundary'

const Project = lazyWithRetry(() => import('./pages/Project'), 'pages/Project')
const Login = lazyWithRetry(() => import('./pages/Login'), 'pages/Login')
const Account = lazyWithRetry(() => import('./pages/Account'), 'pages/Account')
const Credentials = lazyWithRetry(() => import('./pages/Credentials'), 'pages/Credentials')
const Locations = lazyWithRetry(() => import('./pages/Locations'), 'pages/Locations')
const Dashboard = lazyWithRetry(() => import('./pages/Dashboard'), 'pages/Dashboard')
const MissionDetail = lazyWithRetry(() => import('./pages/MissionDetail'), 'pages/MissionDetail')
const TaskDetail = lazyWithRetry(() => import('./pages/TaskDetail'), 'pages/TaskDetail')
const DecisionDetail = lazyWithRetry(() => import('./pages/DecisionDetail'), 'pages/DecisionDetail')
const StrategyMap = lazyWithRetry(() => import('./pages/StrategyMap'), 'pages/StrategyMap')
const Decisions = lazyWithRetry(() => import('./pages/Decisions'), 'pages/Decisions')
const Members = lazyWithRetry(() => import('./pages/Members'), 'pages/Members')

const CommandPalette = lazyWithRetry(() => import('./components/CommandPalette'), 'components/CommandPalette')

function MissionsRouteRedirect({ params }: { params: { projectId: string } }) {
  return <Redirect to={missionsPanelLink(params.projectId)} />
}

const Spinner = () => (
  <div className="flex items-center justify-center h-full">
    <span className="spinner spinner-md" />
  </div>
)

const MOBILE_VIEW_TITLES: Partial<Record<ActiveView, string>> = {
  project: 'Projects',
  dashboard: 'Dashboard',
  missions: 'Missions',
  decisions: 'Decision Log',
  strategy: 'Context Map',
  members: 'Members',
}

/** Mobile-only top bar: hamburger (opens the sidebar drawer) + app icon +
 *  contextual page title + a search button that opens the command palette
 *  (otherwise keyboard-only via Cmd+K, unreachable on touch).
 *  Must live inside SidebarProvider so useSidebar() resolves. */
function MobileTopBar() {
  const { toggleSidebar } = useSidebar()
  const { activeView } = useNavigation()
  const [location] = useLocation()
  const theme = useStore((s) => s.theme)

  const title = location.startsWith('/credentials')
    ? 'Credentials'
    : location.startsWith('/locations')
      ? 'Locations'
    : location.startsWith('/account')
      ? 'Settings'
      : MOBILE_VIEW_TITLES[activeView] ?? 'agen8'

  return (
    <div className="md:hidden shrink-0 flex items-center gap-2 min-h-12 px-2 pt-[env(safe-area-inset-top)] border-b border-[var(--border)] bg-[var(--bg-surface)]">
      <button
        type="button"
        onClick={toggleSidebar}
        aria-label="Open navigation menu"
        className="h-9 w-9 flex items-center justify-center rounded-[8px] border-none bg-transparent cursor-pointer text-[var(--text-2)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors shrink-0"
      >
        <Menu size={18} />
      </button>
      <img
        src={brandIconFor(theme)}
        alt=""
        aria-hidden="true"
        className="w-[22px] h-[22px] shrink-0 rounded-[6px]"
      />
      <span className="flex-1 min-w-0 truncate text-[0.9375rem] font-semibold tracking-[-0.02em] text-[var(--text-1)]">
        {title}
      </span>
      <button
        type="button"
        onClick={() => useStore.getState().setPaletteOpen(true)}
        aria-label="Open search"
        className="h-9 w-9 flex items-center justify-center rounded-[8px] border-none bg-transparent cursor-pointer text-[var(--text-2)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors shrink-0"
      >
        <Search size={18} />
      </button>
    </div>
  )
}

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
  const { paletteOpen, theme, fontFamily, fontScale, resetEphemeral } = useStore()
  const { projectId } = useNavigation()
  const auth = useAuth()
  const [location, navigate] = useLocation()
  const isAuthRoute = location === '/login'

  useEffect(() => {
    document.documentElement.setAttribute('data-theme', theme)
    // Point the iOS home-screen icon at the active theme's tile. Safari reads
    // this link live when "Add to Home Screen" is tapped, so the saved icon
    // matches whatever theme is showing at that moment.
    const link = document.querySelector<HTMLLinkElement>('link[rel="apple-touch-icon"]')
    if (link) link.href = `/apple-touch-icon-${theme}.png`
  }, [theme])

  useEffect(() => {
    const root = document.documentElement
    root.setAttribute('data-font-family', fontFamily)
    root.style.setProperty('--app-font-scale', `${fontScale}px`)
  }, [fontFamily, fontScale])

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
  const prevFocusKey = useRef(`${projectId}`)
  useEffect(() => {
    const key = `${projectId}`
    if (key !== prevFocusKey.current) {
      prevFocusKey.current = key
      resetEphemeral()
    }
  }, [projectId, resetEphemeral])

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
        <div className="flex flex-1 min-h-0 w-full flex-col md:flex-row">
          <a
            href="#main-content"
            className="skip-to-content"
          >
            Skip to content
          </a>
          <MobileTopBar />
          <Sidebar />
          <main id="main-content" className="app-main md:pt-[env(safe-area-inset-top)]">
            <Suspense fallback={<Spinner />}>
              <PageErrorBoundary>
                <Switch>
                  <Route path="/project/:projectId/missions/:missionId" component={MissionDetail} />
                  <Route path="/project/:projectId/tasks/:taskId" component={TaskDetail} />
                  <Route path="/project/:projectId/decisions/:decisionId" component={DecisionDetail} />
                  <Route path="/project/:projectId/missions" component={MissionsRouteRedirect} />
                  <Route path="/project/:projectId/strategy">{(params) => <StrategyMap projectId={params.projectId} />}</Route>
                  <Route path="/project/:projectId/decisions" component={Decisions} />
                  <Route path="/project/:projectId/members" component={Members} />
                  <Route path="/project/:projectId/builder">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId/roles">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId/dashboard" component={Dashboard} />
                  <Route path="/project/:projectId/metrics">{(params) => <Redirect to={`/project/${params.projectId}/dashboard`} />}</Route>
                  <Route path="/project/:projectId" component={Dashboard} />
                  <Route path="/account" component={Account} />
                  <Route path="/credentials" component={Credentials} />
                  <Route path="/locations" component={Locations} />
                  <Route path="/login" component={Login} />
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
