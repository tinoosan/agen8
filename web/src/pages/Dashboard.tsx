import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { useNavigation, type DashboardPanel } from '../lib/routing'
import { useLocation, useSearch } from 'wouter'
import { useProjectSpaces } from '../hooks/useProjectSpaces'
import { useMissions } from '../hooks/useMissions'
import { AlertCircle, RefreshCw, LayoutGrid, Maximize2, Minimize2, PanelRight } from 'lucide-react'
import { Skeleton } from '@/components/ui/skeleton'
import { Button } from '@/components/ui/button'
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs'
import { cn } from '@/lib/utils'
import EscalationQueue from '../components/dashboard/EscalationQueue'
import OpActionQueue from '../components/dashboard/OpActionQueue'
import DecisionFeed from '../components/dashboard/DecisionFeed'
import MissionSummary from '../components/dashboard/MissionSummary'
import SystemPulse from '../components/dashboard/SystemPulse'
import DashboardContextPanel from '../components/dashboard/DashboardContextPanel'
// useCountUp moved to SystemPulse
import { useEscalationSSE, usePendingEscalations } from '../hooks/useEscalations'
import { useOpActionSSE, usePendingOpActions } from '../hooks/useOpActions'
// useOperatorMetrics removed — SystemPulse derives state from metrics + escalation data
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts'
import { useOperatorNotifications } from '../hooks/useOperatorNotifications'
import { useSpaceList } from '../hooks/useSpace'
import { writeStoredDashboardContextCollapsed, readStoredDashboardContextCollapsed } from '../lib/dashboardContextPanelStorage'
import { useAuth } from '../hooks/useAuth'

function readNotificationSoundEnabled(projectId: string | null): boolean {
  if (!projectId) return false
  try {
    return localStorage.getItem(`notification_sound_${projectId}`) === 'true'
  } catch {
    return false
  }
}

/* ── Dashboard page ──────────────────────────────── */

export default function Dashboard() {
  const { focusedProjectRoot, projectId } = useNavigation()
  const [, navigate] = useLocation()
  const rawSearch = useSearch()
  const queryClient = useQueryClient()
  const projectSpacesQuery = useProjectSpaces(projectId)
  const contextSpacesQuery = useProjectSpaces(projectId, { includeDeleted: true, refetchInterval: 10_000 })
  const projectSpaceRecords = projectSpacesQuery.data ?? []
  const contextSpaceRecords = contextSpacesQuery.data ?? projectSpaceRecords
  const spaceLabelByOwnerId = useMemo(() => {
    const out = new Map<string, string>()
    for (const t of contextSpaceRecords) {
      if (t.spaceName && !out.has(t.spaceId)) out.set(t.spaceId, t.spaceName)
    }
    return out
  }, [contextSpaceRecords])

  const spacesQuery = useSpaceList({
    projectId: projectId ?? undefined,
    status: 'open',
    enabled: !!projectId,
    limit: 500,
  })
  const spaces = useMemo(
    () => (spacesQuery.data ?? []).filter(space => {
      const status = (space.status ?? '').trim().toLowerCase()
      return status !== 'archived' && status !== 'deleted'
    }),
    [spacesQuery.data],
  )
  const openSpaceCount = spaces.length
  const hasSpaces = openSpaceCount > 0
  const searchParams = useMemo(() => new URLSearchParams(rawSearch), [rawSearch])
  const rawPanel = searchParams.get('panel')
  const dashboardPanel: DashboardPanel =
    rawPanel === 'missions' || rawPanel === 'actions' || rawPanel === 'decisions' || rawPanel === 'overview'
      ? rawPanel
      : 'overview'
  const rawActionsPanelType = searchParams.get('type')
  const actionsPanelType: 'all' | 'oa' | 'escalation' =
    rawActionsPanelType === 'oa' || rawActionsPanelType === 'escalation' ? rawActionsPanelType : 'all'

  // Deep linking: read ?open=<id> from URL on mount (F34g)
  const [initialOpenId] = useState<string | null>(
    () => new URLSearchParams(window.location.search).get('open'),
  )

  // Clean up URL param after reading it so refresh doesn't re-open
  useEffect(() => {
    if (!initialOpenId) return
    const url = new URL(window.location.href)
    url.searchParams.delete('open')
    window.history.replaceState({}, '', url.toString())
  }, [initialOpenId])

  // SSE-driven cross-surface invalidation (F21)
  useEscalationSSE()
  useOpActionSSE()

  // Pending counts hoisted for dashboard pulse and human-loop queues.
  const pendingEscalationsQuery = usePendingEscalations(projectId)
  const pendingOpActionsQuery = usePendingOpActions(projectId)
  const pendingEscalations = pendingEscalationsQuery.data ?? []
  const pendingOpActions = pendingOpActionsQuery.data ?? []
  const activeMissionsQuery = useMissions(projectId, 'active')
  const activeMissionCount = useMemo(
    () => (activeMissionsQuery.data ?? []).filter(mission => mission.status === 'active').length,
    [activeMissionsQuery.data],
  )
  const hasHumanLoop = pendingEscalations.length > 0 || pendingOpActions.length > 0

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

  // Browser push notifications (F34c / F2 / F3)
  const soundEnabled = readNotificationSoundEnabled(projectId)
  useOperatorNotifications({ enabled: !!projectId, projectId, soundEnabled })
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
        queryClient.invalidateQueries({ queryKey: ['project.space.list'] }),
        queryClient.invalidateQueries({ queryKey: ['space.list'] }),
      ])
    } finally {
      window.setTimeout(() => setManualRefreshing(false), 120)
    }
  }, [focusedProjectRoot, queryClient])

  const setDashboardPanel = useCallback((panel: DashboardPanel) => {
    if (!projectId) return
    const params = new URLSearchParams(rawSearch)
    if (panel === 'overview') params.delete('panel')
    else params.set('panel', panel)
    if (panel !== 'actions') params.delete('type')
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
    if (pendingEscalations.length > 0) return 'attention'
    if (activeMissionCount > 0 || openSpaceCount > 0) return 'active'
    return 'idle'
  }, [activeMissionCount, openSpaceCount, pendingEscalations.length])

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
            title="Refresh spaces"
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
            {/* System Pulse — status sentence + stat strip + ambient glow */}
            <SystemPulse
              spaceCount={openSpaceCount}
              activeMissionCount={activeMissionCount}
              pendingEscalationCount={pendingEscalations.length}
              pendingOACount={pendingOpActions.length}
              escalationUrgencies={pendingEscalations.map(e => e.urgency)}
              focusMode={focusMode}
            />

            {/* Overview grid — only render the column structure when there
                are spaces. Otherwise the empty column labels take up space
                above the empty-state card, creating an asymmetric gap. */}
            {hasSpaces && (
              <div
                className={cn(
                  'dashboard-overview-grid',
                  focusMode && 'dashboard-overview-grid-focus',
                  !contextCollapsed && !focusMode && 'dashboard-overview-grid-context-open',
                )}
              >
                {!focusMode && (
                  <div className="dashboard-main-column">
                    <div className="dashboard-column-label">
                      Work in Motion
                    </div>
                    <div className="dash-stagger dash-stagger-4">
                      <DecisionFeed projectId={projectId} spaceLabelByOwnerId={spaceLabelByOwnerId} spaces={contextSpaceRecords} />
                    </div>
                    <div className="dash-stagger dash-stagger-5">
                      <MissionSummary projectId={projectId} mode="active" />
                    </div>
                  </div>
                )}

                <div className={cn('dashboard-side-column', !hasHumanLoop && !focusMode && 'dashboard-side-column-quiet')}>
                  <div className="dashboard-column-label">
                    Human Loop
                  </div>
                  <div className="dash-stagger dash-stagger-2">
                    <EscalationQueue projectId={projectId} initialSelectedId={initialOpenId} focusMode={focusMode} />
                  </div>
                  <div className="dash-stagger dash-stagger-3">
                    <OpActionQueue projectId={projectId} initialSelectedId={initialOpenId} focusMode={focusMode} />
                  </div>
                  {!hasHumanLoop && !focusMode && (
                    <div className="dashboard-human-loop-empty">
                      <div className="dashboard-human-loop-empty-title">Nothing needs a person right now.</div>
                      <p>
                        Escalations and operator actions will appear here when agents need an approval,
                        policy decision, or manual step.
                      </p>
                    </div>
                  )}
                </div>
              </div>
            )}

            {/* Space sections — hidden in focus mode */}
            {!focusMode && spacesQuery.isLoading && (
              <div className="dashboard-section flex flex-col gap-3">
                {[1, 2].map(i => (
                  <Skeleton key={i} className="rounded-[var(--r-lg)] h-[60px]" />
                ))}
              </div>
            )}

            {!focusMode && spacesQuery.isError && (
              <div className="dashboard-section flex items-center gap-3 px-4 py-3 rounded-lg bg-[var(--bg-surface)] border border-[var(--border)] text-sm">
                <AlertCircle size={15} className="text-[var(--red)] shrink-0" />
                <span className="flex-1 text-[var(--text-2)]">
                  Failed to load spaces.{' '}
                  {spacesQuery.error instanceof Error ? spacesQuery.error.message : 'Unknown error.'}
                </span>
                <button
                  onClick={() => spacesQuery.refetch()}
                  className="flex items-center gap-1 text-xs text-[var(--accent)] hover:underline shrink-0"
                >
                  <RefreshCw size={11} />
                  Retry
                </button>
              </div>
            )}

            {!focusMode && !spacesQuery.isLoading && !spacesQuery.isError && !hasSpaces && (
              <section className="animate-fade-in py-16 text-center flex flex-col items-center gap-3">
                <div className="w-10 h-10 rounded-lg bg-[var(--bg-surface)] flex items-center justify-center">
                  <LayoutGrid size={20} className="text-[var(--text-3)] opacity-40" />
                </div>
                <h3 className="text-[15px] font-semibold text-[var(--text-1)] tracking-[-0.02em] m-0">
                  Create your first space
                </h3>
                <p className="text-[13px] text-[var(--text-3)] max-w-[360px] m-0 leading-relaxed">
                  Spaces are where members coordinate, leave work context, and move shared objectives forward. Start a new space from the sidebar to see activity here.
                </p>
              </section>
            )}
          </div>
        </TabsContent>

        </div>
      </div>
      <DashboardContextPanel
        open={!contextCollapsed}
        panel={dashboardPanel}
        actionType={actionsPanelType}
        projectId={projectId}
        focusedProjectRoot={focusedProjectRoot}
        spaceLabelByOwnerId={spaceLabelByOwnerId}
        spaces={contextSpaceRecords}
        onPanelChange={setDashboardPanel}
      />
      </div>
      </Tabs>
    </div>
    </div>
  )
}
