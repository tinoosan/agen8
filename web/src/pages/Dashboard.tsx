import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigation, type DashboardPanel } from '../lib/routing'
import { useLocation, useSearch } from 'wouter'
import { useMissions } from '../hooks/useMissions'
import { RefreshCw, Maximize2, Minimize2, PanelRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import DecisionFeed from '../components/dashboard/DecisionFeed'
import MissionSummary from '../components/dashboard/MissionSummary'
import DashboardContextPanel from '../components/dashboard/DashboardContextPanel'
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts'
import { writeStoredDashboardContextCollapsed, readStoredDashboardContextCollapsed } from '../lib/dashboardContextPanelStorage'
import { useAuth } from '../hooks/useAuth'

/* ── Dashboard page ──────────────────────────────── */

export default function Dashboard() {
  const { focusedProjectRoot, projectId } = useNavigation()
  const [, navigate] = useLocation()
  const rawSearch = useSearch()
  const queryClient = useQueryClient()
  const searchParams = useMemo(() => new URLSearchParams(rawSearch), [rawSearch])
  const rawPanel = searchParams.get('panel')
  const dashboardPanel: DashboardPanel =
    rawPanel === 'missions' || rawPanel === 'decisions' || rawPanel === 'overview'
      ? rawPanel
      : 'overview'

  const activeMissionsQuery = useMissions(projectId, 'active')
  const activeMissionCount = useMemo(
    () => (activeMissionsQuery.data ?? []).filter(mission => mission.status === 'active').length,
    [activeMissionsQuery.data],
  )

  // Focus mode (F34a) — persisted in localStorage
  const [focusMode, setFocusMode] = useState(() => {
    try { return localStorage.getItem('dashboard.focusMode') === 'true' } catch { return false }
  })
  const [contextCollapsed, setContextCollapsed] = useState(readStoredDashboardContextCollapsed)
  const toggleFocusMode = useCallback(() => {
    setFocusMode(prev => {
      const next = !prev
      try { localStorage.setItem('dashboard.focusMode', String(next)) } catch { /* noop */ }
      return next
    })
  }, [])
  const toggleDashboardContext = useCallback(() => {
    setContextCollapsed(prev => {
      const next = !prev
      writeStoredDashboardContextCollapsed(next)
      return next
    })
  }, [])

  useEffect(() => {
    if (dashboardPanel === 'overview') return
    setContextCollapsed(false)
    writeStoredDashboardContextCollapsed(false)
  }, [dashboardPanel])

  const auth = useAuth()
  const userFirstName = useMemo(() => {
    const rawName = auth.user?.name?.trim() ?? auth.user?.email?.trim() ?? ''
    if (!rawName) return 'there'
    return rawName.split(/\s+/)[0]
  }, [auth.user?.email, auth.user?.name])
  const [manualRefreshing, setManualRefreshing] = useState(false)
  const [mainScrollActive, setMainScrollActive] = useState(false)
  const mainScrollTimeoutRef = useRef<number | null>(null)

  // Keyboard shortcuts (F34d)
  useKeyboardShortcuts({
    onToggleFocus: toggleFocusMode,
  })

  const handleRefresh = useCallback(async () => {
    setManualRefreshing(true)
    try {
      await Promise.all([
        queryClient.invalidateQueries({ queryKey: ['missions'] }),
        queryClient.invalidateQueries({ queryKey: ['decisions'] }),
      ])
    } finally {
      window.setTimeout(() => setManualRefreshing(false), 120)
    }
  }, [queryClient])

  const setDashboardPanel = useCallback((panel: DashboardPanel) => {
    if (!projectId) return
    const params = new URLSearchParams(rawSearch)
    if (panel === 'overview') params.delete('panel')
    else params.set('panel', panel)
    const qs = params.toString()
    navigate(`/project/${encodeURIComponent(projectId)}/dashboard${qs ? `?${qs}` : ''}`)
  }, [navigate, projectId, rawSearch])

  const handleMainScroll = useCallback(() => {
    setMainScrollActive(true)
    if (mainScrollTimeoutRef.current !== null) {
      window.clearTimeout(mainScrollTimeoutRef.current)
    }
    mainScrollTimeoutRef.current = window.setTimeout(() => {
      setMainScrollActive(false)
      mainScrollTimeoutRef.current = null
    }, 900)
  }, [])

  useEffect(() => {
    return () => {
      if (mainScrollTimeoutRef.current !== null) {
        window.clearTimeout(mainScrollTimeoutRef.current)
      }
    }
  }, [])

  // System state — drives ambient visual treatment
  const systemState = useMemo(() => {
    if (activeMissionCount > 0) return 'active'
    return 'idle'
  }, [activeMissionCount])

  if (!projectId) {
    return (
      <div className="flex h-full items-center justify-center p-8 text-center">
        <div>
          <h1 className="text-xl font-semibold text-[var(--text-1)]">Select a project</h1>
          <p className="mt-2 text-sm text-[var(--text-3)]">Agen8 now uses projects as the work boundary.</p>
        </div>
      </div>
    )
  }

  return (
    <div className="dashboard-page" data-state={systemState}>
    <div className="dashboard-shell">
      <Tabs defaultValue="overview" className="dashboard-shell-tabs w-full">
      <div className="dashboard-shell-toolbar mb-5 flex items-center justify-between gap-4">
        <TabsList className="dashboard-hero-tabs h-8 bg-[var(--bg-surface)]">
          <TabsTrigger value="overview" className="dashboard-hero-tab text-[11px] px-3 py-1 data-[state=active]:bg-[var(--bg-elevated)]">Today</TabsTrigger>
        </TabsList>
        <div className="dashboard-hero-actions flex items-center gap-2">
          <Button
            variant="ghost"
            size="sm"
            onClick={toggleFocusMode}
            className={cn(
              'dashboard-hero-utility text-[var(--text-3)] hover:text-[var(--text-1)]',
              focusMode && 'text-[var(--accent)] bg-[var(--accent)]/10',
            )}
            title={focusMode ? 'Exit focus mode (F)' : 'Focus mode (F)'}
          >
            {focusMode ? <Minimize2 size={13} /> : <Maximize2 size={13} />}
            <span>{focusMode ? 'Exit Focus' : 'Focus'}</span>
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => { void handleRefresh() }}
            className="dashboard-hero-utility text-[var(--text-3)] hover:text-[var(--text-1)]"
            title="Refresh"
          >
            <RefreshCw size={13} className={manualRefreshing ? 'animate-spin' : ''} />
            <span>Refresh</span>
          </Button>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            title={contextCollapsed ? 'Show context panel' : 'Hide context panel'}
            aria-label={contextCollapsed ? 'Show context panel' : 'Hide context panel'}
            data-testid="dashboard-context-panel-toggle"
            onClick={toggleDashboardContext}
            className={cn(
              'w-8 h-8 rounded-[10px] transition-colors',
              contextCollapsed
                ? 'text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)]'
                : 'text-[var(--text-1)] bg-[var(--bg-hover)]',
            )}
          >
            <PanelRight size={16} aria-hidden />
          </Button>
        </div>
      </div>
      <div className={cn('dashboard-shell-body', !contextCollapsed && 'dashboard-shell-body-context-open')}>
      <div className={cn('dashboard-main-shell min-w-0 flex-1', !contextCollapsed && 'dashboard-main-shell-context-open')}>
        <div
          className={cn('dashboard-main-scroll', mainScrollActive && 'dashboard-scroll-active')}
          onScroll={handleMainScroll}
        >
        {/* Header */}
        <div className={cn('dashboard-hero mb-8 flex items-start justify-between gap-6', !contextCollapsed && 'dashboard-hero-context-open')}>
          <div className="min-w-0">
            <h1 className="m-0 mt-1 text-[31px] font-semibold tracking-[-0.05em] leading-[1.05] text-[var(--text-1)]">
              Hello {userFirstName}
            </h1>
          </div>
          {focusMode && (
            <div className="text-[12px] text-[var(--text-3)]">
              Only the work waiting on a person.
            </div>
          )}
        </div>

        <TabsContent value="overview" className="mt-0">
          <div className="dashboard-flow">
            <div className="dashboard-main-column">
              <div className="dashboard-column-label">
                Work in Motion
              </div>
              <div className="dash-stagger dash-stagger-4">
                <DecisionFeed projectId={projectId} />
              </div>
              <div className="dash-stagger dash-stagger-5">
                <MissionSummary projectId={projectId} mode="active" />
              </div>
            </div>
          </div>
        </TabsContent>

        </div>
      </div>
      <DashboardContextPanel
        open={!contextCollapsed}
        panel={dashboardPanel}
        projectId={projectId}
        focusedProjectRoot={focusedProjectRoot}
        onPanelChange={setDashboardPanel}
      />
      </div>
      </Tabs>
    </div>
    </div>
  )
}
