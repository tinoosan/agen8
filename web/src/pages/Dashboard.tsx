import { useState, useMemo, useCallback, useEffect, useRef } from 'react'
import { useQueryClient } from '@tanstack/react-query'
import { qk } from '../lib/queryKeys'
import { useNavigation, strategyMapLink, filteredTasksLink, missionsPageLink, decisionsLink } from '../lib/routing'
import { useLocation, useSearch } from 'wouter'
import { useMissions } from '../hooks/useMissions'
import { RefreshCw, Search } from 'lucide-react'
import { Button } from '@/components/ui/button'
import { useStore } from '../lib/store'
import { cn } from '@/lib/utils'
import DecisionFeed from '../components/dashboard/DecisionFeed'
import MissionSummary from '../components/dashboard/MissionSummary'
import DashboardWorkingNow from '../components/dashboard/DashboardWorkingNow'
import NeedsAttention from '../components/dashboard/NeedsAttention'
import GettingStartedCard from '../components/dashboard/GettingStartedCard'
import SinceYouWereAway from '../components/dashboard/SinceYouWereAway'
import DashboardBriefing from '../components/dashboard/DashboardBriefing'
import RecentlyShipped from '../components/dashboard/RecentlyShipped'
import { StrategyMapSearch } from '../components/strategy/StrategyMapSearch'
import { useStrategyGraph } from '../components/strategy/useStrategyGraph'
import { useKeyboardShortcuts } from '../hooks/useKeyboardShortcuts'
import { useAuth } from '../hooks/useAuth'

/* ── Dashboard page ──────────────────────────────── */

export default function Dashboard() {
  const { focusedProjectRoot, projectId } = useNavigation()
  const [, navigate] = useLocation()
  const rawSearch = useSearch()
  const queryClient = useQueryClient()
  const searchParams = useMemo(() => new URLSearchParams(rawSearch), [rawSearch])
  const rawPanel = searchParams.get('panel')

  const activeMissionsQuery = useMissions(projectId, 'active')
  const activeMissionCount = useMemo(
    () => (activeMissionsQuery.data ?? []).filter(mission => mission.status === 'active').length,
    [activeMissionsQuery.data],
  )

  // Context-map node search, rendered in place here so it can be opened from
  // the dashboard without leaving for the map. The graph nodes feed the search
  // list; picking a result deep-links to the map focused on that node.
  const { nodes: searchNodes } = useStrategyGraph(projectId, focusedProjectRoot)
  const searchOpen = useStore((s) => s.strategySearchOpen)
  const setSearchOpen = useStore((s) => s.setStrategySearchOpen)

  // Focus mode (F34a) — persisted in localStorage
  const [focusMode, setFocusMode] = useState(() => {
    try { return localStorage.getItem('dashboard.focusMode') === 'true' } catch { return false }
  })
  const toggleFocusMode = useCallback(() => {
    setFocusMode(prev => {
      const next = !prev
      try { localStorage.setItem('dashboard.focusMode', String(next)) } catch { /* noop */ }
      return next
    })
  }, [])

  // Legacy ?panel= deep links — redirect to the dedicated pages so old
  // bookmarks and notifications that targeted the dashboard rail still resolve.
  useEffect(() => {
    if (!projectId) return
    if (rawPanel === 'tasks') {
      const status = searchParams.get('status')
      navigate(filteredTasksLink(projectId, status ?? 'all'))
    } else if (rawPanel === 'missions') {
      navigate(missionsPageLink(projectId))
    } else if (rawPanel === 'decisions') {
      navigate(decisionsLink(projectId))
    }
  }, [rawPanel, projectId, searchParams, navigate])

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
        queryClient.invalidateQueries({ queryKey: qk.missionsAll }),
        queryClient.invalidateQueries({ queryKey: qk.keyResultsAll }),
        queryClient.invalidateQueries({ queryKey: qk.keyResultsListAllRoot }),
        queryClient.invalidateQueries({ queryKey: qk.keyResultProgressHistoryRoot }),
        queryClient.invalidateQueries({ queryKey: qk.decisionsAll }),
        queryClient.invalidateQueries({ queryKey: qk.decisionLogAll }),
        queryClient.invalidateQueries({ queryKey: qk.tasksBoardAll }),
        queryClient.invalidateQueries({ queryKey: qk.taskGetAll }),
      ])
    } finally {
      window.setTimeout(() => setManualRefreshing(false), 120)
    }
  }, [queryClient])

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
      <div className="dashboard-shell-tabs w-full">
      <div className="dashboard-shell-toolbar mb-5 flex items-center justify-end gap-4">
        <div className="dashboard-hero-actions flex items-center gap-2">
          <Button
            type="button"
            variant="ghost"
            size="icon"
            title="Search the context map"
            aria-label="Search the context map"
            data-testid="dashboard-map-search"
            onClick={() => setSearchOpen(true)}
            className="hidden md:inline-flex w-8 h-8 rounded-[10px] text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)] transition-colors"
          >
            <Search size={16} aria-hidden />
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
        </div>
      </div>
      <div className="dashboard-shell-body">
      <div className="dashboard-main-shell min-w-0 flex-1">
        <div
          className={cn('dashboard-main-scroll', mainScrollActive && 'dashboard-scroll-active')}
          onScroll={handleMainScroll}
        >
        {/* Header — greeting + at-a-glance briefing read as one unit, so the
            hero margin is tight and the vitals line carries its own mb-8. */}
        <div className="dashboard-hero mb-3 flex items-start justify-between gap-6">
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

        <DashboardBriefing projectId={projectId} />

        <GettingStartedCard projectId={projectId} />

        <SinceYouWereAway projectId={projectId} />

        <NeedsAttention projectId={projectId} />

        {/* Two-column standing content. The dashboard's usable width is gated by
            the inline sidebar, so reflow on a CONTAINER query, not the viewport:
            side-by-side once the content area clears ~880px, single column below.
            Banners above stay full-width. */}
        <div className="@container">
          <div className="grid grid-cols-1 items-start gap-x-8 gap-y-8 @min-[880px]:grid-cols-2">
            {/* Left — state of the board. The per-status task counts moved up
                into the hero briefing line, so this column is the live "who's on
                what" plus active missions, not a tile grid. */}
            <div className="flex min-w-0 flex-col gap-8">
              <DashboardWorkingNow projectId={projectId} />
              <MissionSummary projectId={projectId} mode="active" />
            </div>
            {/* Right — what's happened */}
            <div className="flex min-w-0 flex-col gap-8">
              <RecentlyShipped projectId={projectId} />
              <DecisionFeed projectId={projectId} />
            </div>
          </div>
        </div>

        </div>
      </div>
      </div>
      </div>
    </div>
    <StrategyMapSearch
      open={searchOpen}
      onOpenChange={setSearchOpen}
      nodes={searchNodes}
      onSelect={(nodeId) => {
        setSearchOpen(false)
        navigate(strategyMapLink(projectId, nodeId))
      }}
    />
    </div>
  )
}
