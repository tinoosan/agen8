import '@xyflow/react/dist/style.css'

import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import {
  ReactFlow,
  type Node,
  type Edge,
  useReactFlow,
  ReactFlowProvider,
} from '@xyflow/react'
import { Network, GitBranch, CircleCheck, Diamond, Target } from 'lucide-react'
import { useLocation } from 'wouter'
import { missionsPanelLink } from '../lib/routing'
import { Button } from '@/components/ui/button'
import { Skeleton } from '@/components/ui/skeleton'
import { useStrategyGraph } from '../components/strategy/useStrategyGraph'
import { reactFlowNodeTypes, reactFlowEdgeTypes } from '../components/strategy/registry'
import { AnimatePresence } from 'framer-motion'
import { DetailPanel } from '../components/strategy/DetailPanel'
import { StrategyMapHelp } from '../components/strategy/StrategyMapHelp'
import { StrategyMapSearch } from '../components/strategy/StrategyMapSearch'
import { StrategyMapFilterBar } from '../components/strategy/StrategyMapFilterBar'
import {
  computeAttentionFilter,
  computeFailedFilter,
  computeTraceFilter,
  type FilterPreset,
} from '../components/strategy/strategyMapFilters'
import { useNavigation } from '../lib/routing'
import { useStrategyMapStore } from '../components/strategy/strategyMapStore'
import {
  getNextDisplayMode,
  useDeferredStrategyGraph,
  useLeafDisplayPhase,
  useStableSelectionNode,
} from '../components/strategy/strategyMapPerformance'

interface Props {
  projectId: string
}

interface InnerProps extends Props {
  projectRoot: string | null
  nodes: Node[]
  edges: Edge[]
  isLoading: boolean
}

type SavedViewport = { x: number; y: number; zoom: number }

/**
 * Per-project viewport persistence. We key by projectId so switching projects
 * doesn't restore the wrong pan/zoom, and store only the three numbers React
 * Flow needs. Failures are logged but non-fatal — if localStorage is
 * unavailable (private browsing, quota), the map falls back to fit-view and
 * the user loses only the persistence nicety, not functionality.
 */
function viewportStorageKey(projectId: string): string {
  return `agen8:strategyMap:viewport:${projectId}`
}

function loadSavedViewport(projectId: string): SavedViewport | null {
  let raw: string | null
  try {
    raw = localStorage.getItem(viewportStorageKey(projectId))
  } catch (e) {
    console.warn('[StrategyMap] localStorage.getItem failed', e)
    return null
  }
  if (!raw) return null
  let parsed: unknown
  try {
    parsed = JSON.parse(raw)
  } catch (e) {
    console.warn('[StrategyMap] saved viewport parse failed', e)
    return null
  }
  if (
    !parsed ||
    typeof parsed !== 'object' ||
    typeof (parsed as SavedViewport).x !== 'number' ||
    typeof (parsed as SavedViewport).y !== 'number' ||
    typeof (parsed as SavedViewport).zoom !== 'number'
  ) {
    console.warn('[StrategyMap] saved viewport shape invalid, ignoring')
    return null
  }
  const v = parsed as SavedViewport
  return { x: v.x, y: v.y, zoom: v.zoom }
}

function saveViewport(projectId: string, viewport: SavedViewport): void {
  try {
    localStorage.setItem(
      viewportStorageKey(projectId),
      JSON.stringify({ x: viewport.x, y: viewport.y, zoom: viewport.zoom }),
    )
  } catch (e) {
    console.warn('[StrategyMap] localStorage.setItem failed', e)
  }
}

function StrategyMapInner({ projectId, projectRoot, nodes, edges, isLoading }: InnerProps) {
  const ZOOM_EPSILON = 0.0005
  const [selectedNodeId, setSelectedNodeId] = useState<string | null>(null)
  const [focusNodeId, setFocusNodeId] = useState<string | null>(null)
  const [activeFilter, setActiveFilter] = useState<FilterPreset | null>(null)
  const [contextDepth, setContextDepth] = useState(0)
  const [helpOpen, setHelpOpen] = useState(false)
  const [searchOpen, setSearchOpen] = useState(false)
  const { fitView, setCenter, getZoom, zoomIn, zoomOut, setViewport } = useReactFlow()
  // Synchronous load so `defaultViewport` on ReactFlow can use it without a
  // first-frame flash. Rememoizes when the user switches projects.
  const initialViewport = useMemo(
    () => loadSavedViewport(projectId),
    [projectId],
  )
  const hasRestoredRef = useRef(false)
  const interactionReleaseTimerRef = useRef<number | null>(null)
  const zoomReleaseTimerRef = useRef<number | null>(null)
  const displayModeReleaseTimerRef = useRef<number | null>(null)
  const pendingZoomRef = useRef(1)
  const moveStartZoomRef = useRef(1)
  const lastViewportZoomRef = useRef(1)
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

  // Smart filter computation — overrides focus-neighborhood dimming when active.
  const filterResult = useMemo(() => {
    if (!activeFilter) return null
    if (activeFilter === 'attention') return computeAttentionFilter(displayNodes, displayEdges)
    if (activeFilter === 'failed') return computeFailedFilter(displayNodes, displayEdges)
    if (activeFilter === 'trace' && selectedNodeId) {
      const structuralEdges = displayEdges.filter((e) => e.type === 'statusEdge')
      return computeTraceFilter(selectedNodeId, structuralEdges, displayEdges, contextDepth)
    }
    return null
  }, [activeFilter, displayNodes, displayEdges, selectedNodeId, contextDepth])

  // Clear trace filter and reset depth when node is deselected
  useEffect(() => {
    if (!selectedNodeId && activeFilter === 'trace') {
      queueMicrotask(() => {
        setActiveFilter(null)
        setContextDepth(0)
      })
    }
  }, [selectedNodeId, activeFilter])

  // Read ?focus= query param on mount
  const pendingFocusRef = useRef<string | null>(null)
  useEffect(() => {
    const params = new URLSearchParams(window.location.search)
    const focusParam = params.get('focus')
    if (focusParam) {
      pendingFocusRef.current = focusParam
      const url = new URL(window.location.href)
      url.searchParams.delete('focus')
      window.history.replaceState({}, '', url.toString())
    }
  }, [])

  // Watch both the ref (URL param on mount) and the store (command palette while on map)
  const storePendingFocus = useStrategyMapStore((s) => s.pendingFocusNodeId)
  useEffect(() => {
    const targetId = storePendingFocus ?? pendingFocusRef.current
    if (!targetId || displayNodes.length === 0) return
    const targetNode = displayNodes.find(n => n.id === targetId)
    if (targetNode) {
      pendingFocusRef.current = null
      useStrategyMapStore.setState({ pendingFocusNodeId: null })
      queueMicrotask(() => {
        setFocusNodeId(targetNode.id)
        setSelectedNodeId(targetNode.id)
      })
      if (targetNode.position?.x !== undefined && targetNode.position?.y !== undefined) {
        setTimeout(() => {
          setCenter(targetNode.position.x + 110, targetNode.position.y, {
            duration: 600,
            zoom: 0.9,
          })
        }, 100)
      }
    }
  }, [displayNodes, setCenter, storePendingFocus])

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
      if (displayModeReleaseTimerRef.current != null) {
        window.clearTimeout(displayModeReleaseTimerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    if (displayNodes.length === 0 || hasRestoredRef.current) return
    hasRestoredRef.current = true
    if (initialViewport) {
      // Restore the user's saved pan/zoom instantly — setViewport without a
      // duration option is synchronous and animation-free, so there is no
      // jitter between the default viewport and the restored one.
      setViewport(initialViewport)
      return
    }
    // First-time visit (or cleared storage) — animate a welcome fit-to-all.
    const t = setTimeout(() => fitView({ padding: 0.18, duration: 800 }), 80)
    return () => clearTimeout(t)
  }, [displayNodes.length, fitView, setViewport, initialViewport])

  // Switching projects within the same mounted component needs to re-trigger
  // the restore-or-fit path. Resetting the guard allows the effect above to
  // run a second time with the new project's initialViewport.
  useEffect(() => {
    hasRestoredRef.current = false
  }, [projectId])

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

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (
        document.activeElement?.tagName === 'INPUT' ||
        document.activeElement?.tagName === 'TEXTAREA' ||
        (document.activeElement as HTMLElement)?.isContentEditable
      ) {
        return
      }

      // If either modal (help or search) is open, let shadcn Dialog handle
      // its own dismiss keys (Esc, etc). Don't fire any strategy-map
      // shortcuts while the modal owns focus.
      if (helpOpen || searchOpen) return

      if (e.key === 'Escape') {
        setSelectedNodeId(null)
        setFocusNodeId(null)
        return
      }

      // ? opens the keyboard-shortcut help overlay.
      if (e.key === '?') {
        e.preventDefault()
        setHelpOpen(true)
        return
      }

      // / opens the command-palette-style node search. Also accept
      // Cmd/Ctrl+K as a secondary binding for IDE-style muscle memory.
      if (e.key === '/' || ((e.metaKey || e.ctrlKey) && e.key === 'k')) {
        e.preventDefault()
        setSearchOpen(true)
        return
      }

      // Zoom in / out via keyboard. Accept both `+` (with shift) and `=`
      // (unshifted on US keyboards) as "zoom in" since that's how every
      // other app does it. `-` / `_` is "zoom out".
      if (e.key === '+' || e.key === '=') {
        e.preventDefault()
        zoomIn({ duration: 200 })
        return
      }
      if (e.key === '-' || e.key === '_') {
        e.preventDefault()
        zoomOut({ duration: 200 })
        return
      }

      // ── Smart filter shortcuts ─────────────────────────────────────
      // A = toggle Attention filter
      if (e.key === 'a' || e.key === 'A') {
        e.preventDefault()
        setActiveFilter((f) => f === 'attention' ? null : 'attention')
        return
      }
      // X = toggle Failed filter
      if (e.key === 'x' || e.key === 'X') {
        e.preventDefault()
        setActiveFilter((f) => f === 'failed' ? null : 'failed')
        return
      }
      // T = toggle Trace Path (only when a node is selected)
      if ((e.key === 't' || e.key === 'T') && selectedNodeId) {
        e.preventDefault()
        setActiveFilter((f) => f === 'trace' ? null : 'trace')
        return
      }
      // [ / ] handled in capture-phase handler above

      // Shift+F fits the entire map into the viewport (global reset).
      // Plain F fits just the current cluster (handled further down).
      if ((e.key === 'f' || e.key === 'F') && e.shiftKey) {
        e.preventDefault()
        fitView({ padding: 0.18, duration: 600 })
        return
      }

      // Enter / Space activates the currently focused node (opens its
      // detail panel). Idempotent when the panel is already open; a
      // no-op when nothing is focused.
      if (e.key === 'Enter' || e.key === ' ') {
        if (effectiveFocusNodeId) {
          e.preventDefault()
          setSelectedNodeId(effectiveFocusNodeId)
        }
        return
      }

      // F key = fit the current cluster (or the whole map if no focus) into
      // the viewport. This is the "travel to the far-away nodes" affordance —
      // when leaves drift to the outer edges of a cluster, pressing F zooms
      // out just enough to see the entire cluster including the outliers,
      // without having to manually pan and scroll.
      if (e.key === 'f' || e.key === 'F') {
        e.preventDefault()
        const currentNode = effectiveFocusNodeId ? nodeById.get(effectiveFocusNodeId) : null
        const currentCluster = (currentNode?.data as { clusterColor?: string } | undefined)?.clusterColor

        if (currentCluster) {
          const clusterNodes = displayNodes
            .filter(
              (n) =>
                (n.data as { clusterColor?: string } | undefined)?.clusterColor === currentCluster,
            )
            .map((n) => ({ id: n.id }))
          if (clusterNodes.length > 0) {
            fitView({ padding: 0.18, duration: 600, nodes: clusterNodes })
            return
          }
        }
        // No focus, or focused node has no cluster: fit everything.
        fitView({ padding: 0.18, duration: 600 })
        return
      }

      // Tab / arrow key on an empty map: focus the first mission so the user
      // can start keyboard-traversing the graph without needing to click.
      // This is the "how do I even start navigating?" entry point.
      if (!effectiveFocusNodeId) {
        if (
          e.key === 'Tab' ||
          e.key === 'ArrowUp' ||
          e.key === 'ArrowDown' ||
          e.key === 'ArrowLeft' ||
          e.key === 'ArrowRight'
        ) {
          e.preventDefault()
          if (missionsSorted.length > 0) {
            const first = missionsSorted[0]
            setFocusNodeId(first.id)
            setSelectedNodeId(first.id)
            markInteraction(450)
            if (first.position?.x !== undefined && first.position?.y !== undefined) {
              setCenter(first.position.x + 110, first.position.y, {
                duration: 400,
                zoom: Math.max(getZoom(), 0.8),
              })
            }
          }
        }
        return
      }

      // Tab / Shift+Tab cycles between mission clusters (cross-cluster hop).
      // This is the "switch to another cluster" affordance — arrow keys stay
      // within the current cluster, so Tab is how you leave one.
      if (e.key === 'Tab') {
        e.preventDefault()
        if (missionsSorted.length === 0) return

        const currentNode = nodeById.get(effectiveFocusNodeId)
        const currentCluster = (currentNode?.data as { clusterColor?: string } | undefined)?.clusterColor
        const currentMissionIdx = missionsSorted.findIndex((m) => {
          const missionCluster = (m.data as { clusterColor?: string } | undefined)?.clusterColor
          return missionCluster === currentCluster
        })

        const step = e.shiftKey ? -1 : 1
        const nextIdx = currentMissionIdx === -1
          ? 0
          : (currentMissionIdx + step + missionsSorted.length) % missionsSorted.length
        const nextMission = missionsSorted[nextIdx]

        setFocusNodeId(nextMission.id)
        setSelectedNodeId(nextMission.id)
        markInteraction(450)
        if (nextMission.position?.x !== undefined && nextMission.position?.y !== undefined) {
          setCenter(nextMission.position.x + 110, nextMission.position.y, {
            duration: 400,
            zoom: Math.max(getZoom(), 0.8),
          })
        }
        return
      }

      if (['ArrowUp', 'ArrowDown', 'ArrowLeft', 'ArrowRight'].includes(e.key)) {
        e.preventDefault()

        // Spatial arrow key navigation CONSTRAINED TO THE CURRENT CLUSTER.
        // Arrow keys keep you inside the mission cluster you're already in —
        // to hop to another cluster, press Tab (or Shift+Tab). This prevents
        // arrow keys from accidentally jumping across the whole map to a
        // node in an unrelated cluster.
        //
        // Algorithm (in-cluster only):
        //   1. Filter candidates to nodes with the same clusterColor as
        //      the focused node.
        //   2. For every such candidate, compute the vector from the focused
        //      node to the candidate.
        //   3. Require the candidate to be inside a ±60° cone of the pressed
        //      direction (so "right" ignores nodes that are mostly below
        //      but slightly right).
        //   4. Score = euclidean distance × (1 + angular deviation factor).
        //      Nearest dead-center candidate wins.
        const currentNode = nodeById.get(effectiveFocusNodeId)
        if (!currentNode || currentNode.position == null) return

        const currentCluster = (currentNode.data as { clusterColor?: string } | undefined)?.clusterColor
        if (!currentCluster && !filterResult) return

        const cx = currentNode.position.x
        const cy = currentNode.position.y

        const targetDirection = (() => {
          switch (e.key) {
            case 'ArrowRight': return { x: 1, y: 0 }
            case 'ArrowLeft':  return { x: -1, y: 0 }
            case 'ArrowDown':  return { x: 0, y: 1 }   // +y is down in screen coords
            case 'ArrowUp':    return { x: 0, y: -1 }
            default: return { x: 0, y: 0 }
          }
        })()

        const CONE_HALF_ANGLE = Math.PI / 3 // 60 degrees each side

        let bestId: string | null = null
        let bestScore = Infinity

        for (const candidate of displayNodes) {
          if (candidate.id === effectiveFocusNodeId) continue
          if (candidate.position == null) continue

          // Filter constraint: when a filter is active, only navigate
          // between highlighted nodes. Otherwise, stay in the same cluster.
          if (filterResult) {
            if (!filterResult.nodeIds.has(candidate.id)) continue
          } else {
            const candidateCluster = (candidate.data as { clusterColor?: string } | undefined)?.clusterColor
            if (candidateCluster !== currentCluster) continue
          }

          const dx = candidate.position.x - cx
          const dy = candidate.position.y - cy
          const distance = Math.hypot(dx, dy)
          if (distance < 1) continue

          const normDx = dx / distance
          const normDy = dy / distance
          const dot = normDx * targetDirection.x + normDy * targetDirection.y
          if (dot < Math.cos(CONE_HALF_ANGLE)) continue

          const angularPenalty = 1 + (1 - dot) * 2
          const score = distance * angularPenalty

          if (score < bestScore) {
            bestScore = score
            bestId = candidate.id
          }
        }

        if (bestId) {
          const nextNode = nodeById.get(bestId)
          if (nextNode) {
            setFocusNodeId(bestId)
            setSelectedNodeId(bestId)
            markInteraction(450)

            if (nextNode.position?.x !== undefined && nextNode.position?.y !== undefined) {
              setCenter(nextNode.position.x + 110, nextNode.position.y, {
                duration: 400,
                zoom: Math.max(getZoom(), 0.8),
              })
            }
          }
        }
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [effectiveFocusNodeId, displayNodes, nodeById, missionsSorted, setCenter, getZoom, markInteraction, fitView, zoomIn, zoomOut, helpOpen, searchOpen, filterResult, selectedNodeId])

  const handleEdgeClick = useCallback((_: React.MouseEvent, edge: Edge) => {
    // Navigate to the other end of the edge relative to the focused node.
    // If no node is focused, default to the target (child) end.
    const goToId = effectiveFocusNodeId === edge.target ? edge.source
      : effectiveFocusNodeId === edge.source ? edge.target
      : edge.target
    const goToNode = nodeById.get(goToId)
    if (goToNode) {
      setFocusNodeId(goToNode.id)
      setSelectedNodeId(goToNode.id)
      markInteraction(450)
      if (goToNode.position?.x !== undefined && goToNode.position?.y !== undefined) {
        setCenter(goToNode.position.x + 110, goToNode.position.y, {
          duration: 400,
          zoom: Math.max(getZoom(), 0.8)
        })
      }
    }
  }, [effectiveFocusNodeId, nodeById, setCenter, getZoom, markInteraction])

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Assertively lock focus. Clicking a node should never randomly deselect it if clicked twice,
    // which protects the Double-Click Zoom flow from destroying the Slide-Over state!
    setSelectedNodeId(node.id)
    setFocusNodeId(node.id)
  }, [])

  const handleNodeDoubleClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Assertive cinematic push to closely inspect node details
    // The +110 offsets the viewport perfectly allowing the DetailPanel slide-over to coexist!
    markInteraction(450)
    setCenter((node.position?.x ?? 0) + 110, (node.position?.y ?? 0), { zoom: 1.6, duration: 400 })
  }, [setCenter, markInteraction])


  const handlePaneClick = useCallback(() => {
    setSelectedNodeId(null)
    setFocusNodeId(null)
  }, [])

  const handlePaneDoubleClick = useCallback((evt: React.MouseEvent) => {
    const target = evt.target as HTMLElement
    // Only trigger cinematic macro pull-out if they explicitly double-clicked the raw canvas
    // and bypass if the event bubbled up from a Node or Edge!
    if (target.closest('.react-flow__node') || target.closest('.react-flow__edge')) {
      return
    }

    markInteraction(650)
    setSelectedNodeId(null)
    setFocusNodeId(null)
    fitView({ padding: 0.18, duration: 600 })
  }, [fitView, markInteraction])

  // ── Highlighting logic (1-hop neighborhood focus) ─────────────────────
  // When a node is focused, we compute a visual hierarchy:
  //   - neighborhood nodes  = focused node + every 1-hop neighbor (both
  //                           structural and context-link edges)
  //   - direct edges        = edges touching the focused node
  //   - neighborhood edges  = edges where BOTH endpoints are in the
  //                           neighborhood (so parent↔child within the
  //                           focused subgraph stay visible)
  // Everything outside the neighborhood dims (non-neighborhood nodes fade,
  // non-neighborhood edges drop to 0.3 opacity) so the user gets an
  // uncluttered view of just the relationships of the node they clicked.
  //
  // Naming note: `clusterNodeIds` / `clusterEdgeIds` kept the old names for
  // backwards compatibility with leaf-node components, but the SEMANTICS
  // changed from "same cluster colour" to "focus neighborhood".
  const { directEdgeIds, clusterNodeIds, clusterEdgeIds } = useMemo(() => {
    // Smart filter overrides focus-neighborhood dimming
    if (filterResult) {
      return {
        directEdgeIds: filterResult.edgeIds,
        clusterNodeIds: filterResult.nodeIds,
        clusterEdgeIds: filterResult.edgeIds,
      }
    }

    if (!effectiveFocusNodeId) {
      return { directEdgeIds: null, clusterNodeIds: null, clusterEdgeIds: null }
    }

    const neighborIds = new Set<string>([effectiveFocusNodeId])
    const dEdges = new Set<string>()

    for (const edge of displayEdges) {
      if (edge.source === effectiveFocusNodeId) {
        neighborIds.add(edge.target)
        dEdges.add(edge.id)
      } else if (edge.target === effectiveFocusNodeId) {
        neighborIds.add(edge.source)
        dEdges.add(edge.id)
      }
    }

    const cEdges = new Set<string>(dEdges)
    for (const edge of displayEdges) {
      if (neighborIds.has(edge.source) && neighborIds.has(edge.target)) {
        cEdges.add(edge.id)
      }
    }

    return { directEdgeIds: dEdges, clusterNodeIds: neighborIds, clusterEdgeIds: cEdges }
  }, [effectiveFocusNodeId, displayEdges, filterResult])

  // Mirror React-owned render state into the Zustand store so node/edge
  // components can subscribe to individual fields via selectors. See
  // strategyMapStore.ts for the rationale — previously this was a single
  // React Context whose value changed on every interaction flip, forcing a
  // full re-render of every consuming node. The store's selector model
  // invalidates only subscriptions whose derived output actually changed.
  useEffect(() => {
    useStrategyMapStore.setState({
      displayMode,
      leafPhase,
      isInteracting,
      isZooming,
      isDense,
      focusNodeId: effectiveFocusNodeId,
      selectedNodeId,
      clusterNodeIds,
      directEdgeIds,
      clusterEdgeIds,
      activeFilter,
    })
  }, [
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
  ])

  // Capture-phase handler for keys that ReactFlow may swallow.
  // Fires before any element can stopPropagation.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (helpOpen || searchOpen) return

      if (e.key === 'Escape') {
        if (activeFilter) {
          e.stopPropagation()
          setActiveFilter(null)
          return
        }
        if (selectedNodeId || focusNodeId) {
          e.stopPropagation()
          setSelectedNodeId(null)
          setFocusNodeId(null)
        }
        return
      }

      // Context depth [ / ] — must be in capture phase to beat ReactFlow
      if (e.key === '[' && activeFilter === 'trace') {
        e.preventDefault()
        e.stopPropagation()
        setContextDepth((d) => Math.max(0, d - 1))
        return
      }
      if (e.key === ']' && activeFilter === 'trace') {
        e.preventDefault()
        e.stopPropagation()
        setContextDepth((d) => Math.min(3, d + 1))
        return
      }
    }
    window.addEventListener('keydown', handler, true)
    return () => window.removeEventListener('keydown', handler, true)
  }, [selectedNodeId, focusNodeId, activeFilter, helpOpen, searchOpen])

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
          onFilterChange={setActiveFilter}
          hasSelectedNode={!!selectedNodeId}
          matchCount={filterResult?.matchCount ?? 0}
          contextDepth={contextDepth}
          onContextDepthChange={setContextDepth}
        />

          <ReactFlow
            nodes={displayNodes}
            edges={displayEdges}
            nodeTypes={reactFlowNodeTypes}
            edgeTypes={reactFlowEdgeTypes}
            onNodeClick={handleNodeClick}
            onNodeDoubleClick={handleNodeDoubleClick}
            onEdgeClick={handleEdgeClick}
            onPaneClick={handlePaneClick}
            onMoveStart={(_, viewport) => {
              pendingZoomRef.current = viewport.zoom
              moveStartZoomRef.current = viewport.zoom
              lastViewportZoomRef.current = viewport.zoom
              if (displayModeReleaseTimerRef.current != null) {
                window.clearTimeout(displayModeReleaseTimerRef.current)
                displayModeReleaseTimerRef.current = null
              }
              markInteraction(240)
            }}
            onMove={(_, viewport) => {
              const previousZoom = lastViewportZoomRef.current
              pendingZoomRef.current = viewport.zoom
              lastViewportZoomRef.current = viewport.zoom
              if (Math.abs(viewport.zoom - previousZoom) > ZOOM_EPSILON) {
                markZooming()
              }
              markInteraction(240)
            }}
            onMoveEnd={(_, viewport) => {
              pendingZoomRef.current = viewport.zoom
              if (Math.abs(viewport.zoom - moveStartZoomRef.current) > ZOOM_EPSILON) {
                markZooming()
              }
              if (displayModeReleaseTimerRef.current != null) {
                window.clearTimeout(displayModeReleaseTimerRef.current)
              }
              displayModeReleaseTimerRef.current = window.setTimeout(() => {
                setDisplayMode(current => getNextDisplayMode(pendingZoomRef.current, current))
                displayModeReleaseTimerRef.current = null
              }, 120)
              markInteraction(180)
              // Persist the resting viewport so navigating away and back
              // doesn't lose the user's place on large maps.
              saveViewport(projectId, {
                x: viewport.x,
                y: viewport.y,
                zoom: viewport.zoom,
              })
            }}
            onNodeDragStart={() => markInteraction(240)}
            onNodeDrag={() => markInteraction(240)}
            onNodeDragStop={() => markInteraction(180)}
            nodesDraggable={true}
            nodesConnectable={false}
            elementsSelectable={true}
            panOnScroll={false}
            selectionOnDrag={false}
            panOnDrag={true}
            zoomOnScroll={true}
            zoomOnPinch={true}
            zoomOnDoubleClick={false}
            onlyRenderVisibleElements
            // When we have a saved viewport, hand it to React Flow as the
            // *default* so the first paint already shows the correct pan/zoom
            // — no flash. The useEffect above still calls setViewport once
            // as a belt-and-suspenders, but defaultViewport eliminates the
            // one-frame flicker that a pure effect-driven restore would have.
            // When nothing is saved, fall back to the built-in `fitView` prop
            // which the welcome-fit effect then refines with 0.18 padding.
            {...(initialViewport
              ? { defaultViewport: initialViewport }
              : { fitView: true })}
            minZoom={0.1}
            maxZoom={2.5}
            proOptions={{ hideAttribution: true }}
            style={{ background: 'var(--color-bg)' }}
          />

        {effectiveFocusNodeId && (
          <div className="absolute bottom-4 left-1/2 -translate-x-1/2 z-10 pointer-events-none">
            <span className="text-[0.625rem] text-muted-foreground/60 px-2.5 py-1">
              Esc to clear focus
            </span>
          </div>
        )}

        {/* Dynamic Legend */}
        <div
          className="absolute bottom-5 left-5 z-10 pointer-events-none flex flex-col gap-2 p-[14px] rounded-[8px]"
          style={{
            background: 'color-mix(in srgb, var(--bg-panel) 86%, transparent)',
            boxShadow: '0 10px 28px rgba(0,0,0,0.08)',
            opacity: 0.92,
          }}
        >
          <h4 className="text-[0.5625rem] font-bold text-foreground/50 uppercase tracking-[0.6px] mb-1">Map Legend</h4>
          <div className="flex flex-col gap-2.5">
            <div className="flex items-center gap-3">
              <Target size={13} className="text-[var(--accent)]" strokeWidth={2.2} />
              <span className="text-[0.71875rem] font-medium text-foreground tracking-tight">Mission</span>
            </div>
            <div className="flex items-center gap-3 w-full">
              <div className="w-[14px] h-[9px] flex items-center justify-start rounded-[2.5px] border border-[#0071e3]/60 bg-[#0071e3]/10 overflow-hidden relative left-[1px]">
                 <div className="h-full w-[8px] bg-[#0071e3]/80" />
              </div>
              <span className="text-[0.71875rem] font-medium text-foreground tracking-tight pl-[3px]">Key Result</span>
            </div>
            <div className="flex items-center gap-3">
              <CircleCheck size={13} className="text-[var(--text-3)]" strokeWidth={2.2} />
              <span className="text-[0.71875rem] font-medium text-foreground tracking-tight">Task</span>
            </div>
            <div className="flex items-center gap-3">
              <Diamond size={13} className="text-[#0071e3]" strokeWidth={2.2} />
              <span className="text-[0.71875rem] font-medium text-foreground tracking-tight">Decision</span>
            </div>
          </div>
        </div>

        <AnimatePresence>
          {selectedNode && (
            <DetailPanel
              node={selectedNode}
              projectId={projectId}
              projectRoot={projectRoot}
              onClose={() => {
                setSelectedNodeId(null)
                setFocusNodeId(null)
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
  const { nodes, edges, isLoading } = useStrategyGraph(projectId, focusedProjectRoot, { showArchived })
  const isEmpty = !isLoading && nodes.length === 0

  return (
    <div className="flex flex-col h-full" style={{ background: 'var(--color-bg)' }}>
      <div className="flex items-center gap-2.5 px-5 py-3 border-b border-border/50 shrink-0"
        style={{ background: 'var(--color-bg)' }}>
        <Network size={16} className="text-muted-foreground/60" />
        <h1 className="text-sm font-medium text-foreground/70">Strategy Map</h1>
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
          />
        </ReactFlowProvider>
      )}
    </div>
  )
}
