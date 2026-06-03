import { useSpace, useSpaceMemberList } from '../hooks/useSpace'
import { useNavigation } from '../lib/routing'
import ContextPanel from '../components/ContextPanel'
import { useSpaceStatus } from '../hooks/useSpaceStatus'
import { lazyWithRetry } from '../lib/lazyWithRetry'
import type { SpaceMember, Task } from '../lib/types'
import { Coins, Cpu, PanelRight, LayoutGrid, KanbanSquare } from 'lucide-react'

// Tab icons keyed by tab name. Defined alongside the tab definitions so
// Overview/Chat/Board/Inspector/Schedule render with a consistent
// 12px Lucide glyph before the label — improves scan time.
const TAB_ICONS = {
  overview: LayoutGrid,
  board: KanbanSquare,
} as const

const TAB_LABELS = {
  overview: 'Overview',
  board: 'Board',
} as const
import { Tabs, TabsList, TabsTrigger, TabsContent } from '@/components/ui/tabs'
import { Button } from '@/components/ui/button'
import { Suspense, useState, useRef, useCallback, useEffect, useMemo, type PointerEvent as ReactPointerEvent } from 'react'
import { useLocation, useSearch } from 'wouter'
import { formatCost, formatTokens } from '../lib/format'
import { cn } from '@/lib/utils'
import { onContextPanelOpenFile } from '../lib/contextPanelEvents'
import { isRemovedSpaceMember } from '../lib/removedMemberLogs'
const SpaceOverviewTab = lazyWithRetry(() => import('../components/space-focus/SpaceOverviewTab'), 'components/space-focus/SpaceOverviewTab')
const SpaceBoardTab = lazyWithRetry(() => import('../components/space-focus/SpaceBoardTab'), 'components/space-focus/SpaceBoardTab')

const CONTEXT_COLLAPSED_KEY = 'oa-context-panel-collapsed'
const CONTEXT_WIDTH_KEY = 'oa-context-panel-width'
const CONTEXT_COLLAPSED_WIDTH = 0
const CONTEXT_DEFAULT_WIDTH = 720
const CONTEXT_MIN_WIDTH = 360
const CONTEXT_MAX_WIDTH = 2200
// Width kept for the main column so a dragged-wide panel never swallows the
// whole window; the effective drag limit shrinks with the viewport.
const CONTEXT_MIN_MAIN_WIDTH = 360

function TabLoadingFallback() {
  return (
    <div className="flex h-full items-center justify-center text-[var(--text-3)]">
      <span className="spinner spinner-sm" />
    </div>
  )
}

function clampContextWidth(value: number, availableWidth?: number): number {
  if (!Number.isFinite(value)) return CONTEXT_DEFAULT_WIDTH
  const available = typeof availableWidth === 'number' && Number.isFinite(availableWidth)
    ? Math.max(0, availableWidth)
    : typeof window === 'undefined'
      ? CONTEXT_MAX_WIDTH + CONTEXT_MIN_MAIN_WIDTH
      : window.innerWidth
  const maxPanelWidth = Math.min(CONTEXT_MAX_WIDTH, Math.max(0, available - CONTEXT_MIN_MAIN_WIDTH))
  const minPanelWidth = Math.min(CONTEXT_MIN_WIDTH, maxPanelWidth)
  return Math.max(minPanelWidth, Math.min(maxPanelWidth, Math.round(value)))
}

function readContextCollapsed(): boolean {
  try { return localStorage.getItem(CONTEXT_COLLAPSED_KEY) !== 'false' } catch { return true }
}
function writeContextCollapsed(v: boolean): void {
  try { localStorage.setItem(CONTEXT_COLLAPSED_KEY, String(v)) } catch { /* ignore */ }
}
function readContextWidth(): number {
  try {
    const raw = localStorage.getItem(CONTEXT_WIDTH_KEY)
    if (!raw) return CONTEXT_DEFAULT_WIDTH
    return clampContextWidth(Number(raw))
  } catch {
    return CONTEXT_DEFAULT_WIDTH
  }
}
function writeContextWidth(v: number): void {
  try { localStorage.setItem(CONTEXT_WIDTH_KEY, String(clampContextWidth(v))) } catch { /* ignore */ }
}

/**
 * Valid SpaceFocus tabs. Hoisted out of the component so the
 * localStorage restore helpers below can validate the stored value
 * without re-creating the Set on every render.
 */
const VALID_TABS: ReadonlySet<string> = new Set([
  'overview',
  'board',
])

/**
 * Per-space tab memory. Storing the last-used tab per spaceId means
 * navigating back to a space (even after a reload or from another
 * page) restores the tab the user was on. Shared links with an
 * explicit ?tab=X still win — the restore only runs when the URL
 * has no tab param at all.
 */
function spaceTabStorageKey(spaceId: string): string {
  return `oa-space-tab:${spaceId || 'default'}`
}

function readStoredSpaceTab(spaceId: string): string | null {
  try {
    const raw = localStorage.getItem(spaceTabStorageKey(spaceId))
    if (raw && VALID_TABS.has(raw)) return raw
  } catch { /* private browsing etc. */ }
  return null
}

function writeStoredSpaceTab(spaceId: string, tab: string): void {
  if (!VALID_TABS.has(tab)) return
  try { localStorage.setItem(spaceTabStorageKey(spaceId), tab) } catch { /* ignore */ }
}

interface SpaceFocusProps {
  /** Stable space ID — set when navigating via /space/:spaceId routes. */
  spaceId: string
}

export default function SpaceFocus({ spaceId: spaceIdProp }: SpaceFocusProps) {
  const [, navigate] = useLocation()
  const { projectId: routeProjectId } = useNavigation()
  // The space ID is always the URL prop — source of truth on /space/ routes.
  const spaceId = spaceIdProp
  // Fetch space-scoped state — planMode here is the effective execution mode
  // for this specific space.
  const spaceQuery = useSpace(spaceId)

  const statusQuery = useSpaceStatus(spaceId)
  const status = statusQuery.data

  // Members for this space. Chat is scoped to one member at a time; the
  // member record owns the chat routing address exposed as channelId.
  const membersQuery = useSpaceMemberList({ spaceId, enabled: !!spaceId, includeRemoved: true })
  const members = useMemo(
    () => (membersQuery.data ?? []).filter((member: SpaceMember) =>
      !isRemovedSpaceMember(member),
    ),
    [membersQuery.data],
  )
  void members
  // Base path for tab/channel URL navigation.
  const focusBasePath = routeProjectId
    ? `/project/${encodeURIComponent(routeProjectId)}/space/${encodeURIComponent(spaceIdProp)}`
    : null

  // ContextPanel (right sidebar) — closed by default, last state persisted.
  const [contextCollapsed, setContextCollapsed] = useState(readContextCollapsed)
  const [contextWidth, setContextWidth] = useState(readContextWidth)
  const [contextFileOpenRequest, setContextFileOpenRequest] = useState<null | { id: number; path: string }>(null)
  const [contextTaskOpenRequest, setContextTaskOpenRequest] = useState<null | { id: number; task: Task; status: string }>(null)
  const contextRequestCounterRef = useRef(0)
  const contextSplitRef = useRef<HTMLDivElement | null>(null)
  const contextRailRef = useRef<HTMLDivElement | null>(null)
  const resizeFrameRef = useRef<number | null>(null)
  const resizeCleanupRef = useRef<null | (() => void)>(null)
  const splitWidthRef = useRef<number | null>(null)
  const widthRef = useRef(contextWidth)

  useEffect(() => {
    widthRef.current = contextWidth
  }, [contextWidth])

  useEffect(() => {
    return () => {
      if (resizeCleanupRef.current) resizeCleanupRef.current()
      if (resizeFrameRef.current !== null) {
        cancelAnimationFrame(resizeFrameRef.current)
        resizeFrameRef.current = null
      }
    }
  }, [])

  const clampContextWidthForCurrentLayout = useCallback((value: number) => {
    const splitWidth = contextSplitRef.current?.clientWidth
    return clampContextWidth(value, splitWidth && splitWidth > 0 ? splitWidth : undefined)
  }, [])

  useEffect(() => {
    if (contextCollapsed) {
      splitWidthRef.current = null
      return
    }
      const preserveContextWidth = () => {
      const splitWidth = contextSplitRef.current?.clientWidth ?? 0
      setContextWidth((previous) => {
        const previousSplitWidth = splitWidthRef.current
        const adjusted = previousSplitWidth !== null && splitWidth > 0
          ? previous + (splitWidth - previousSplitWidth)
          : previous
        const next = clampContextWidth(adjusted, splitWidth > 0 ? splitWidth : undefined)
        splitWidthRef.current = splitWidth > 0 ? splitWidth : previousSplitWidth
        widthRef.current = next
        if (contextRailRef.current) contextRailRef.current.style.width = `${next}px`
        if (next !== previous) writeContextWidth(next)
        return next
      })
    }

    preserveContextWidth()
    const split = contextSplitRef.current
    if (typeof ResizeObserver === 'undefined' || !split) {
      window.addEventListener('resize', preserveContextWidth)
      return () => window.removeEventListener('resize', preserveContextWidth)
    }

    const observer = new ResizeObserver(preserveContextWidth)
    observer.observe(split)
    return () => observer.disconnect()
  }, [contextCollapsed])

  // Tab state from URL query param (?tab=overview|board|inspector).
  // Plan and files are no longer top-level tabs. Removed tab names are
  // ignored rather than redirected; the URL is space-first and only valid
  // space tabs participate in routing state.
  const searchString = useSearch()
  // `null` when the URL has no tab param at all — we use that to
  // distinguish "bare space URL, restore last tab" from "explicit tab
  // specified in URL, honor it". The `|| 'overview'` default from
  // the previous revision collapsed both cases into "overview" and
  // blocked the restore path.
  const rawTabParam = new URLSearchParams(searchString).get('tab')
  const boardTaskParam = new URLSearchParams(searchString).get('task')
  const tabFromURL = rawTabParam && VALID_TABS.has(rawTabParam) ? rawTabParam : null
  const activeTab = tabFromURL ?? 'overview'
  const setActiveTab = useCallback((tab: string) => {
    if (!focusBasePath) return
    // Persist the chosen tab per-space so navigating back (even
    // after a page reload or revisit from a different section)
    // restores where the user left off. See restore effect below.
    writeStoredSpaceTab(spaceId, tab)
    navigate(tab === 'overview' ? focusBasePath : `${focusBasePath}?tab=${tab}`)
  }, [focusBasePath, navigate, spaceId])
  const handleTabPointerDown = useCallback((tab: string, event: ReactPointerEvent<HTMLButtonElement>) => {
    if (event.button !== 0 || event.metaKey || event.ctrlKey || event.altKey || event.shiftKey) return
    if (tab === activeTab) return
    setActiveTab(tab)
  }, [activeTab, setActiveTab])

  // Restore the remembered tab when landing on a bare focus URL
  // (no ?tab= param). Only fires once per spaceId change; explicit
  // URLs always win so shared links still go to the intended tab.
  useEffect(() => {
    if (rawTabParam || !focusBasePath) return
    const remembered = readStoredSpaceTab(spaceId)
    if (!remembered || remembered === 'overview') return
    navigate(`${focusBasePath}?tab=${remembered}`, { replace: true })
  }, [rawTabParam, focusBasePath, navigate, spaceId])

  // Write the current tab to storage whenever it changes via URL
  // (bookmarks, shared links, back/forward navigation). Without this
  // the restore hook only kicks in for tabs you clicked explicitly;
  // arriving via a link wouldn't update the remembered tab.
  useEffect(() => {
    if (tabFromURL) writeStoredSpaceTab(spaceId, tabFromURL)
  }, [tabFromURL, spaceId])

  useEffect(() => {
    return onContextPanelOpenFile((request) => {
      const path = request.path.trim()
      if (!path) return
      setContextCollapsed(false)
      writeContextCollapsed(false)
      contextRequestCounterRef.current += 1
      setContextFileOpenRequest({ id: contextRequestCounterRef.current, path })
    })
  }, [])

  const beginResizeContextPanel = useCallback((event: ReactPointerEvent<HTMLDivElement>) => {
    if (event.button !== 0 || contextCollapsed) return
    event.preventDefault()
    if (resizeCleanupRef.current) resizeCleanupRef.current()

    const startX = event.clientX
    const startWidth = widthRef.current
    let pendingWidth = startWidth
    const rail = contextRailRef.current
    if (!rail) return
    rail.style.transition = 'none'

    const queueWidthUpdate = (nextWidth: number) => {
      pendingWidth = clampContextWidthForCurrentLayout(nextWidth)
      if (resizeFrameRef.current !== null) return
      resizeFrameRef.current = requestAnimationFrame(() => {
        resizeFrameRef.current = null
        widthRef.current = pendingWidth
        if (!contextRailRef.current) return
        contextRailRef.current.style.width = `${pendingWidth}px`
      })
    }

    const cleanup = () => {
      if (resizeFrameRef.current !== null) {
        cancelAnimationFrame(resizeFrameRef.current)
        resizeFrameRef.current = null
      }
      const finalWidth = clampContextWidthForCurrentLayout(pendingWidth)
      widthRef.current = finalWidth
      setContextWidth((previous) => (previous === finalWidth ? previous : finalWidth))
      writeContextWidth(finalWidth)
      if (contextRailRef.current) {
        contextRailRef.current.style.width = `${finalWidth}px`
        contextRailRef.current.style.transition = ''
      }
      document.body.style.cursor = ''
      document.body.style.userSelect = ''
      window.removeEventListener('pointermove', onMove)
      window.removeEventListener('pointerup', onUp)
      window.removeEventListener('pointercancel', onUp)
      resizeCleanupRef.current = null
    }

    const onMove = (moveEvent: PointerEvent) => {
      const delta = startX - moveEvent.clientX
      queueWidthUpdate(startWidth + delta)
    }

    const onUp = () => cleanup()

    document.body.style.cursor = 'col-resize'
    document.body.style.userSelect = 'none'
    window.addEventListener('pointermove', onMove)
    window.addEventListener('pointerup', onUp)
    window.addEventListener('pointercancel', onUp)
    resizeCleanupRef.current = cleanup
  }, [clampContextWidthForCurrentLayout, contextCollapsed])

  // Show a skeleton while space scope is resolving.
  // All hooks above have been called unconditionally so this early return is safe.
  if (spaceQuery.isLoading && !spaceId) {
    return (
      <div className="flex h-full items-center justify-center">
        <span className="spinner spinner-md" />
      </div>
    )
  }

  return (
    <Tabs value={activeTab} onValueChange={setActiveTab} className="flex h-full relative flex-col">
        {/* Space header — compact controls row above the tabs. */}
        <div className="shrink-0 border-b border-[color-mix(in_srgb,var(--border)_50%,transparent)]">
          <div className="space-focus-header-row px-6 py-2 flex items-center gap-3 flex-wrap">
            <TabsList className="h-auto bg-transparent gap-0 p-0 rounded-none shrink-0">
              {(['overview', 'board'] as const).map(tab => {
                const Icon = TAB_ICONS[tab]
                return (
                  <TabsTrigger
                    key={tab}
                    value={tab}
                    onPointerDown={(event) => handleTabPointerDown(tab, event)}
                    className="relative text-[12px] font-medium px-3 py-1.5 rounded-none bg-transparent text-[var(--text-3)] data-[state=active]:text-[var(--text-1)] data-[state=active]:shadow-none after:absolute after:bottom-0 after:left-0 after:right-0 after:h-[2px] after:bg-transparent data-[state=active]:after:bg-[var(--accent)] flex items-center gap-1.5"
                  >
                    <Icon size={12} strokeWidth={1.75} aria-hidden className="shrink-0" />
                    {TAB_LABELS[tab]}
                  </TabsTrigger>
                )
              })}
          </TabsList>
          {/* Optional spend/token stats stay right-aligned next to the context toggle. */}
          <div className="flex-1 min-w-0 flex items-center justify-end gap-3 flex-wrap text-[11px] tabular-nums">
            {status !== undefined && status.totalCostUSD > 0 && (
              <span className="inline-flex items-center gap-1 text-[var(--amber)] font-semibold">
                <Coins size={11} aria-hidden />
                {formatCost(status.totalCostUSD)}
              </span>
            )}
            {status !== undefined && status.totalTokens > 0 && (
              <span className="inline-flex items-center gap-1 text-[var(--text-3)]">
                <Cpu size={11} aria-hidden />
                {formatTokens(status.totalTokens)}
              </span>
            )}
          </div>

          <div className="relative shrink-0 flex items-center gap-1.5">
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  title={contextCollapsed ? 'Show context panel' : 'Hide context panel'}
                  aria-label={contextCollapsed ? 'Show context panel' : 'Hide context panel'}
                  data-testid="context-panel-toggle"
                  onClick={() => {
                    const nextCollapsed = !contextCollapsed
                    setContextCollapsed(nextCollapsed)
                    writeContextCollapsed(nextCollapsed)
                  }}
                  className={cn(
                    'relative w-8 h-8 rounded-[10px] overflow-hidden transition-[color,background,box-shadow] duration-300',
                    contextCollapsed
                      ? 'text-[var(--text-3)] hover:text-[var(--text-1)] hover:bg-[var(--bg-hover)]'
                      : 'text-[var(--text-1)] bg-[var(--bg-hover)] shadow-[inset_0_0_0_1px_color-mix(in_oklab,var(--accent)_28%,transparent)]',
                  )}
                >
                  <span
                    aria-hidden
                    className={cn(
                      'absolute right-1.5 top-1/2 h-4 w-0.5 -translate-y-1/2 rounded-full bg-[var(--accent)] transition-[opacity,transform] duration-300',
                      contextCollapsed ? 'opacity-0 scale-y-50' : 'opacity-100 scale-y-100',
                    )}
                  />
                  <PanelRight
                    size={16}
                    aria-hidden
                    className={cn(
                      'transition-transform duration-300 ease-[cubic-bezier(0.22,1,0.36,1)]',
                      contextCollapsed ? 'translate-x-0' : '-translate-x-0.5',
                    )}
                  />
                </Button>
              </div>
          </div>
        </div>

      <div ref={contextSplitRef} className="flex flex-1 min-h-0 relative">
        {/* Main inspection area */}
        <div
          className="flex-1 min-w-0 flex flex-col"
          style={{ minWidth: contextCollapsed ? 0 : `min(${CONTEXT_MIN_MAIN_WIDTH}px, 100%)` }}
        >
          {/* Content panes */}
          <TabsContent value="overview" className="flex-1 min-h-0 mt-0 overflow-auto">
            <Suspense fallback={<TabLoadingFallback />}>
              <SpaceOverviewTab
                spaceId={spaceId}
                projectId={routeProjectId}
                status={status}
              />
            </Suspense>
          </TabsContent>

          <TabsContent value="board" className="flex-1 min-h-0 mt-0 overflow-hidden">
            <Suspense fallback={<TabLoadingFallback />}>
              <SpaceBoardTab
                spaceId={spaceId}
                initialTaskId={boardTaskParam ?? undefined}
                onOpenTask={(task, status) => {
                  setContextTaskOpenRequest({
                    id: ++contextRequestCounterRef.current,
                    task,
                    status,
                  })
                  setContextCollapsed(false)
                  writeContextCollapsed(false)
                }}
              />
            </Suspense>
          </TabsContent>

      </div>

      {/* Right sidebar — collapsible embedded rail with width + content transitions.
          Width animation runs on its own compositor layer (`will-change`) so
          expanding/collapsing stays smooth while the main chat keeps scrolling. */}
      {spaceId && (
        <div
          ref={contextRailRef}
          className={cn(
            'shrink-0 min-h-0 overflow-hidden relative transition-[width] duration-300 ease-[cubic-bezier(0.22,1,0.36,1)] will-change-[width]',
            contextCollapsed ? 'border-l-0' : 'border-l border-[color-mix(in_srgb,var(--border)_50%,transparent)]',
          )}
          style={{ width: contextCollapsed ? CONTEXT_COLLAPSED_WIDTH : contextWidth }}
        >
          {!contextCollapsed && (
            <div
              className="absolute left-0 top-0 bottom-0 w-3 -translate-x-1/2 cursor-col-resize z-20 group"
              onPointerDown={beginResizeContextPanel}
              aria-hidden
            >
              <div className="absolute left-1/2 top-1/2 h-18 w-[2px] -translate-x-1/2 -translate-y-1/2 rounded-full bg-transparent group-hover:bg-[var(--border)] transition-colors duration-150" />
            </div>
          )}
          <div
            className={cn(
              'absolute inset-0 transition-[opacity,transform] duration-220 ease-out',
              contextCollapsed ? 'opacity-0 translate-x-2 pointer-events-none' : 'opacity-100 translate-x-0',
            )}
          >
            <ContextPanel
              spaceId={spaceId}
              projectId={routeProjectId}
              fileOpenRequest={contextFileOpenRequest}
              taskOpenRequest={contextTaskOpenRequest}
              onFileOpenRequestHandled={(id) => {
                setContextFileOpenRequest((current) => {
                  if (!current || current.id !== id) return current
                  return null
                })
              }}
              onTaskOpenRequestHandled={(id) => {
                setContextTaskOpenRequest((current) => {
                  if (!current || current.id !== id) return current
                  return null
                })
              }}
            />
          </div>
        </div>
      )}
      </div>
    </Tabs>
  )
}
