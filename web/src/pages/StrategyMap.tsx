import '@xyflow/react/dist/style.css'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  type Node,
  type Edge,
  useReactFlow,
  ReactFlowProvider,
} from '@xyflow/react'
import { Network, GitBranch } from 'lucide-react'
import { useLocation } from 'wouter'
import { missionsPanelLink, useNavigation } from '../lib/routing'
import { useStore } from '../lib/store'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useStrategyGraph } from '../components/strategy/useStrategyGraph'
import { AnimatePresence } from 'framer-motion'
import { DetailPanel } from '../components/strategy/DetailPanel'
import { StrategyMapHelp } from '../components/strategy/StrategyMapHelp'
import { StrategyMapSearch } from '../components/strategy/StrategyMapSearch'
import { StrategyMapFilterBar } from '../components/strategy/StrategyMapFilterBar'
import { StrategyMapLegend } from '../components/strategy/StrategyMapLegend'
import { StrategyMapZoomControls } from '../components/strategy/StrategyMapZoomControls'
import { StrategyMapCanvas } from '../components/strategy/StrategyMapCanvas'
import { type FilterPreset, TRACE_INITIAL_DEPTH } from '../components/strategy/strategyMapFilters'
import {
  getNextDisplayMode,
  useDeferredStrategyGraph,
  useLeafDisplayPhase,
  useStableSelectionNode,
} from '../components/strategy/strategyMapPerformance'
import { useStrategyMapViewport } from '../components/strategy/useStrategyMapViewport'
import { useStrategyMapLenses } from '../components/strategy/useStrategyMapLenses'
import { useStrategyMapFocusEffects } from '../components/strategy/useStrategyMapFocusEffects'
import { useStrategyMapKeyboardNav } from '../components/strategy/useStrategyMapKeyboardNav'
import { useStrategyMapNodeHandlers } from '../components/strategy/useStrategyMapNodeHandlers'

interface Props {
  projectId: string
}

interface InnerProps extends Props {
  projectRoot: string | null
  nodes: Node[]
  edges: Edge[]
  isLoading: boolean
  showArchived: boolean
  onRequestShowArchived: () => void
}

function StrategyMapInner({ projectId, projectRoot, nodes, edges, isLoading, showArchived, onRequestShowArchived }: InnerProps) {
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null)
  const [activeFilter, setActiveFilter] = useState<FilterPreset | null>(null)
  const [contextDepth, setContextDepth] = useState(0)
  const [helpOpen, setHelpOpen] = useState(false)
  // Search-open lives in the global store so search can be opened from outside
  // this page — the mobile top-bar button and the dashboard search icon both
  // set the flag (and the dashboard routes here) so arriving on the map lands
  // with the same node-search panel the "/" shortcut opens. The modal Radix
  // dialog clears the flag itself on close (Escape / outside-click / select),
  // so there is no stale-open to guard against. We deliberately do NOT reset
  // the flag on unmount: that cleanup fires during StrictMode's dev
  // mount→unmount→remount and would slam the just-opened panel shut on arrival.
  const searchOpen = useStore((s) => s.strategySearchOpen)
  const setSearchOpen = useStore((s) => s.setStrategySearchOpen)
  const { fitView, setCenter, getZoom, zoomIn, zoomOut, setViewport } = useReactFlow()

  const interactionReleaseTimerRef = useRef<number | null>(null)
  const zoomReleaseTimerRef = useRef<number | null>(null)
  const [displayMode, setDisplayMode] = useState(() =>
    getNextDisplayMode(1, { missionKR: 'full', leaf: 'full' }),
  )
  const [isInteracting, setIsInteracting] = useState(false)
  const [isZooming, setIsZooming] = useState(false)
  const graphSnapshot = useMemo(() => ({ nodes, edges, isLoading }), [nodes, edges, isLoading])
  const deferredGraph = useDeferredStrategyGraph(graphSnapshot, isInteracting)
  const displayNodes = deferredGraph.nodes
  const displayEdges = deferredGraph.edges
  const leafPhase = useLeafDisplayPhase(displayMode.leaf)
  const isDense = displayNodes.length >= 100
  const hasSelectedNode = selectedNodeId ? displayNodes.some(node => node.id === selectedNodeId) : false
  const selectedNode = useStableSelectionNode(hasSelectedNode ? selectedNodeId : null, displayNodes)
  const effectiveFocusNodeId = focusNodeId && displayNodes.some(node => node.id === focusNodeId)
    ? focusNodeId
    : null

  // Restore saved pan/zoom (or welcome-fit on first visit) once nodes load.
  const { initialViewport } = useStrategyMapViewport({
    projectId,
    displayNodeCount: displayNodes.length,
    fitView,
    setViewport,
  })

  // Highlight lenses + 1-hop focus-neighborhood dimming sets.
  const {
    filterResult,
    directEdgeIds,
    clusterNodeIds,
    clusterEdgeIds,
  } = useStrategyMapLenses({
    activeFilter,
    displayNodes,
    displayEdges,
    contextDepth,
    effectiveFocusNodeId,
  })

  // Clear trace and reset depth when the focus cursor clears (e.g. a pane
  // click). Trace anchors on the cursor now, not the panel — so closing the
  // panel while a node stays focused must keep the trace alive.
  useEffect(() => {
    if (!effectiveFocusNodeId && activeFilter === 'trace') {
      queueMicrotask(() => {
        setActiveFilter(null)
        setContextDepth(0)
      })
    }
  }, [effectiveFocusNodeId, activeFilter])

  // Toggling a filter from the bar. Entering trace seeds the first ring so the
  // map lights the selected node's immediate neighbours (depth 0 would show only
  // the selected node — useless); the +/- stepper then walks the rings outward.
  const handleFilterChange = useCallback((filter: FilterPreset | null) => {
    setActiveFilter(filter)
    if (filter === 'trace') setContextDepth(TRACE_INITIAL_DEPTH)
  }, [])

  const markInteraction = useCallback((settleDelay = 180) => {
    setIsInteracting(true)
    if (interactionReleaseTimerRef.current != null) {
      window.clearTimeout(interactionReleaseTimerRef.current)
    }
    interactionReleaseTimerRef.current = window.setTimeout(() => {
      setIsInteracting(false)
      interactionReleaseTimerRef.current = null
    }, settleDelay)
  }, [])

  const markZooming = useCallback((settleDelay = 220) => {
    setIsZooming(true)
    if (zoomReleaseTimerRef.current != null) {
      window.clearTimeout(zoomReleaseTimerRef.current)
    }
    zoomReleaseTimerRef.current = window.setTimeout(() => {
      setIsZooming(false)
      zoomReleaseTimerRef.current = null
    }, settleDelay)
  }, [])

  useEffect(() => {
    return () => {
      if (interactionReleaseTimerRef.current != null) {
        window.clearTimeout(interactionReleaseTimerRef.current)
      }
      if (zoomReleaseTimerRef.current != null) {
        window.clearTimeout(zoomReleaseTimerRef.current)
      }
    }
  }, [])

  // Hot-path indexes used by keyboard navigation, click handlers, and the
  // F-key cluster fit. Building them once per displayNodes change (instead
  // of re-filtering/sorting on every keydown) makes the keydown handler's
  // work O(1) for lookups and avoids per-keypress allocations.
  const nodeById = useMemo(() => {
    const m = new Map<string, Node>()
    for (const n of displayNodes) m.set(n.id, n)
    return m
  }, [displayNodes])

  const missionsSorted = useMemo(
    () =>
      displayNodes
        .filter((n) => n.type === 'mission')
        .sort((a, b) => (a.position?.x ?? 0) - (b.position?.x ?? 0)),
    [displayNodes],
  )

  useStrategyMapKeyboardNav({
    effectiveFocusNodeId,
    displayNodes,
    nodeById,
    missionsSorted,
    setCenter,
    getZoom,
    markInteraction,
    fitView,
    zoomIn,
    zoomOut,
    helpOpen,
    searchOpen,
    filterResult,
    selectedNodeId,
    focusNodeId,
    activeFilter,
    setSelectedNodeId,
    setFocusNodeId,
    setHelpOpen,
    setSearchOpen,
    setActiveFilter,
    setContextDepth,
  })

  const {
    handleEdgeClick,
    handleNodeClick,
    handleNodeDoubleClick,
    handlePaneClick,
    handlePaneDoubleClick,
  } = useStrategyMapNodeHandlers({
    effectiveFocusNodeId,
    selectedNodeId,
    nodeById,
    setCenter,
    getZoom,
    fitView,
    markInteraction,
    setFocusNodeId,
    setSelectedNodeId,
  })

  // Deep-link focus (?focus=) + mirror render state into the
  // Zustand store so node/edge components can subscribe to individual fields.
  useStrategyMapFocusEffects({
    displayNodes,
    setCenter,
    setFocusNodeId,
    setSelectedNodeId,
    displayMode,
    leafPhase,
    isInteracting,
    isZooming,
    isDense,
    effectiveFocusNodeId,
    selectedNodeId,
    clusterNodeIds,
    directEdgeIds,
    clusterEdgeIds,
    activeFilter,
    showArchived,
    onRequestShowArchived,
  })

  return (
    <div className="flex flex-1 min-h-0">
        {/*
          Note: the `.heat-nebula { transform: translateZ(0) }` rule was
          removed deliberately. Forcing GPU layer promotion on 100+ blurred
          elements overflowed Chromium's composite-layer memory budget and
          made the compositor thrash. Without the promotion hint, the browser
          promotes selectively and the frame pacing is dramatically better.
        */}
        <style>{`
          @keyframes leaf-node-enter {
            from { opacity: 0; transform: scale(0.6); }
            to { opacity: 1; transform: scale(1); }
          }
          @keyframes context-dash-flow {
            from { stroke-dashoffset: 16; }
            to { stroke-dashoffset: 0; }
          }
        `}</style>
      <div className="relative flex-1 min-w-0 overflow-hidden" onDoubleClick={handlePaneDoubleClick}>
        {deferredGraph.isLoading && displayNodes.length === 0 && (
          <div className="absolute inset-0 z-10 flex items-center justify-center p-8 pointer-events-none">
            <div className="flex items-center gap-16">
              {[0, 1, 2].map(i => (
                <Skeleton key={i} className="w-20 h-20 rounded-full" />
              ))}
            </div>
          </div>
        )}

        <StrategyMapFilterBar
          activeFilter={activeFilter}
          onFilterChange={handleFilterChange}
          hasFocusedNode={!!effectiveFocusNodeId}
          matchCount={filterResult?.matchCount ?? 0}
          contextDepth={contextDepth}
          onContextDepthChange={setContextDepth}
          onOpenSearch={() => setSearchOpen(true)}
        />

        <StrategyMapCanvas
          nodes={displayNodes}
          edges={displayEdges}
          onNodeClick={handleNodeClick}
          onNodeDoubleClick={handleNodeDoubleClick}
          onEdgeClick={handleEdgeClick}
          onPaneClick={handlePaneClick}
          markInteraction={markInteraction}
          markZooming={markZooming}
          setDisplayMode={setDisplayMode}
          projectId={projectId}
          initialViewport={initialViewport}
        />

        {effectiveFocusNodeId && (
          <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 pointer-events-none">
            <span className="text-[0.625rem] text-muted-foreground/60 px-2.5 py-1">
              Esc to clear focus
            </span>
          </div>
        )}

        <StrategyMapLegend />
        <StrategyMapZoomControls />
        <AnimatePresence>
          {selectedNode && (
            <DetailPanel
              node={selectedNode}
              projectId={projectId}
              projectRoot={projectRoot}
              onClose={() => {
                // Close the panel but keep the focus cursor so traversal can
                // continue with the panel closed. A second Escape (handled in
                // the keyboard hook) clears the cursor.
                setSelectedNodeId(null)
              }}
            />
          )}
        </AnimatePresence>
      </div>

      <StrategyMapHelp open={helpOpen} onOpenChange={setHelpOpen} />
      <StrategyMapSearch
        open={searchOpen}
        onOpenChange={setSearchOpen}
        nodes={displayNodes}
        onSelect={(nodeId) => {
          const node = displayNodes.find((n) => n.id === nodeId)
          if (!node) return
          setFocusNodeId(nodeId)
          setSelectedNodeId(nodeId)
          markInteraction(450)
          if (node.position?.x !== undefined && node.position?.y !== undefined) {
            setCenter(node.position.x + 110, node.position.y, {
              duration: 400,
              zoom: Math.max(getZoom(), 0.8),
            })
          }
        }}
      />
    </div>
  )
}

function EmptyState({ projectId }: Props) {
  const [, navigate] = useLocation()
  return (
    <div className="flex flex-col items-center justify-center flex-1 gap-4 text-center px-8">
      <div className="w-12 h-12 rounded-full bg-muted flex items-center justify-center">
        <GitBranch size={22} className="text-muted-foreground" />
      </div>
      <div>
        <p className="text-sm font-medium text-foreground">No missions yet</p>
        <p className="text-xs text-muted-foreground mt-1">
          Create your first mission to start building the strategy map.
        </p>
      </div>
      <Button
        size="sm"
        variant="outline"
        onClick={() => navigate(missionsPanelLink(projectId))}
      >
        Go to Missions
      </Button>
    </div>
  )
}

export default function StrategyMap({ projectId }: Props) {
  const { focusedProjectRoot } = useNavigation()
  const [showArchived, setShowArchived] = useState(() => {
    try {
      return window.localStorage.getItem(`strategy:${projectId}:showArchived`) === 'true'
    } catch {
      return false
    }
  })
  useEffect(() => {
    try {
      window.localStorage.setItem(`strategy:${projectId}:showArchived`, showArchived ? 'true' : 'false')
    } catch {
      // Local UI preference only.
    }
  }, [projectId, showArchived])
  const enableArchived = useCallback(() => setShowArchived(true), [])
  const { nodes, edges, isLoading } = useStrategyGraph(projectId, focusedProjectRoot, { showArchived })
  const isEmpty = !isLoading && nodes.length === 0

  return (
    <div className="flex flex-col h-full" style={{ background: 'var(--color-bg)' }}>
      <div className="flex items-center gap-2.5 px-5 py-3 border-b border-border/50 shrink-0"
        style={{ background: 'var(--color-bg)' }}>
        <Network size={16} className="text-muted-foreground/60 hidden md:block" />
        <h1 className="text-sm font-medium text-foreground/70 hidden md:block">Context Map</h1>
        <Button
          type="button"
          variant={showArchived ? 'secondary' : 'ghost'}
          size="xs"
          className="ml-auto"
          onClick={() => setShowArchived((value) => !value)}
        >
          Show archived
        </Button>
      </div>

      {isEmpty ? (
        <EmptyState projectId={projectId} />
      ) : (
        <ReactFlowProvider>
          <StrategyMapInner
            projectId={projectId}
            projectRoot={focusedProjectRoot}
            nodes={nodes}
            edges={edges}
            isLoading={isLoading}
            showArchived={showArchived}
            onRequestShowArchived={enableArchived}
          />
        </ReactFlowProvider>
      )}
    </div>
  )
}
