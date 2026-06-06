import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigation, type DashboardPanel } from '../lib/routing'
import { useLocation, useSearch } from 'wouter'
import { useMissions } from '../hooks/useMissions'
import { RefreshCw, PanelRight } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { cn } from '@/lib/utils'
import DecisionFeed from '../components/dashboard/DecisionFeed'
import MissionSummary from '../components/dashboard/MissionSummary'
import TaskSummary from '../components/dashboard/TaskSummary'
import DashboardContextPanel from '../components/dashboard/DashboardContextPanel'
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts'
import { writeStoredDashboardContextCollapsed, readStoredDashboardContextCollapsed } from '../lib/dashboardContextPanelStorage'
import { useAuth } from '../hooks/useAuth'
import { useIsBelow } from '../hooks/use-mobile'

/* Below this width the context panel becomes an on-demand overlay drawer instead of an inline rail. */
const CONTEXT_OVERLAY_BREAKPOINT = 1280

/* ── Dashboard page ──────────────────────────────── */

export default function Dashboard() {
  const { focusedProjectRoot, projectId } = useNavigation()
  const [, navigate] = useLocation()
  const rawSearch = useSearch()
  const queryClient = useQueryClient()
  const searchParams = useMemo(() => new URLSearchParams(rawSearch), [rawSearch])
  const rawPanel = searchParams.get('panel')
  const dashboardPanel: DashboardPanel =
    rawPanel === 'decisions' ? 'decisions' : rawPanel === 'tasks' ? 'tasks' : 'missions'

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
  const isContextOverlay = useIsBelow(CONTEXT_OVERLAY_BREAKPOINT)
  const [contextDrawerOpen, setContextDrawerOpen] = useState(false)
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
  const handleToggleContext = useCallback(() => {
    if (isContextOverlay) {
      setContextDrawerOpen(prev => !prev)
    } else {
      toggleDashboardContext()
    }
  }, [isContextOverlay, toggleDashboardContext])

  useEffect(() => {
    if (rawPanel !== 'missions' && rawPanel !== 'decisions' && rawPanel !== 'tasks') return
    setContextCollapsed(false)
    writeStoredDashboardContextCollapsed(false)
    setContextDrawerOpen(true)
  }, [rawPanel])

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
        queryClient.invalidateQueries({ queryKey: ['mission.list'] }),
        queryClient.invalidateQueries({ queryKey: ['keyResult.list'] }),
        queryClient.invalidateQueries({ queryKey: ['keyResult.listAll'] }),
        queryClient.invalidateQueries({ queryKey: ['keyResult.progressHistory'] }),
        queryClient.invalidateQueries({ queryKey: ['decision.list'] }),
        queryClient.invalidateQueries({ queryKey: ['decision.log'] }),
        queryClient.invalidateQueries({ queryKey: ['project.tasks.board'] }),
        queryClient.invalidateQueries({ queryKey: ['task.get'] }),
      ])
    } finally {
      window.setTimeout(() => setManualRefreshing(false), 120)
    }
  }, [queryClient])

  const setDashboardPanel = useCallback((panel: DashboardPanel) => {
    if (!projectId) return
    const params = new URLSearchParams(rawSearch)
    params.set('panel', panel)
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

  const contextPanelVisible = isContextOverlay ? contextDrawerOpen : !contextCollapsed
  const inlineContextOpen = !isContextOverlay && !contextCollapsed

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
      <div className="dashboard-shell-tabs w-full">
      <div className="dashboard-shell-toolbar mb-5 flex items-center justify-end gap-4">
        <div className="dashboard-hero-actions flex items-center gap-2">
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
            title={contextPanelVisible ? 'Hide context panel' : 'Show context panel'}
            aria-label={contextPanelVisible ? 'Hide context panel' : 'Show context panel'}
            data-testid="dashboard-context-panel-toggle"
            onClick={handleToggleContext}
            className={cn(
              'inline-flex w-8 h-8 rounded-[10px] transition-colors',
              contextPanelVisible
                ? 'text-[var(--text-1)] bg-[var(--bg-hover)]'
                : 'text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)]',
            )}
          >
            <PanelRight size={16} aria-hidden />
          </Button>
        </div>
      </div>
      <div className={cn('dashboard-shell-body', inlineContextOpen && 'dashboard-shell-body-context-open')}>
      <div className={cn('dashboard-main-shell min-w-0 flex-1', inlineContextOpen && 'dashboard-main-shell-context-open')}>
        <div
          className={cn('dashboard-main-scroll', mainScrollActive && 'dashboard-scroll-active')}
          onScroll={handleMainScroll}
        >
        {/* Header */}
        <div className={cn('dashboard-hero mb-8 flex items-start justify-between gap-6', inlineContextOpen && 'dashboard-hero-context-open')}>
          <div className="min-w-0">
            <h1 className="m-0 mt-1 text-[1.9375rem] font-semibold tracking-[-0.05em] leading-[1.05] text-[var(--text-1)]">
              Hello {userFirstName}
            </h1>
          </div>
          {focusMode && (
            <div className="text-[0.75rem] text-[var(--text-3)]">
              Only the work waiting on a person.
            </div>
          )}
        </div>

        <div className="mb-8">
          <TaskSummary projectId={projectId} />
        </div>

        <div className="mt-0">
          <div className="dashboard-flow">
            <div className="dashboard-main-column">
              <div className="dash-stagger dash-stagger-4">
                <DecisionFeed projectId={projectId} />
              </div>
              <div className="dash-stagger dash-stagger-5">
                <MissionSummary projectId={projectId} mode="active" />
              </div>
            </div>
          </div>
        </div>

        </div>
      </div>
      <DashboardContextPanel
        open={contextPanelVisible}
        overlay={isContextOverlay}
        panel={dashboardPanel}
        projectId={projectId}
        focusedProjectRoot={focusedProjectRoot}
        onPanelChange={setDashboardPanel}
        onOpenChange={setContextDrawerOpen}
      />
      </div>
      </div>
    </div>
    </div>
  )
}
