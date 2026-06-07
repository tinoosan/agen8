import { useEffect } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { Node } from '@xyflow/react'
import type { FilterPreset } from './strategyMapFilters'
import type { FitViewFn, GetZoomFn, SetCenterFn, ZoomFn } from './strategyMapControls'

/**
 * All keyboard interaction for the strategy map: the bubble-phase handler
 * (selection/zoom/filter shortcuts + spatial arrow/Tab navigation) and the
 * capture-phase handler for keys ReactFlow would otherwise swallow (Escape,
 * and the `[` / `]` context-depth controls while tracing).
 */
export function useStrategyMapKeyboardNav({
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
}: {
  effectiveFocusNodeId: string | null
  displayNodes: Node[]
  nodeById: Map<string, Node>
  missionsSorted: Node[]
  setCenter: SetCenterFn
  getZoom: GetZoomFn
  markInteraction: (settleDelay?: number) => void
  fitView: FitViewFn
  zoomIn: ZoomFn
  zoomOut: ZoomFn
  helpOpen: boolean
  searchOpen: boolean
  filterResult: { nodeIds: ReadonlySet<string> } | null
  selectedNodeId: string | null
  focusNodeId: string | null
  activeFilter: FilterPreset | null
  setSelectedNodeId: Dispatch<SetStateAction<string | null>>
  setFocusNodeId: Dispatch<SetStateAction<string | null>>
  setHelpOpen: Dispatch<SetStateAction<boolean>>
  // Store-backed action (open the node search), so it is a plain setter rather
  // than a React state dispatcher — the hook only ever calls it with a boolean.
  setSearchOpen: (open: boolean) => void
  setActiveFilter: Dispatch<SetStateAction<FilterPreset | null>>
  setContextDepth: Dispatch<SetStateAction<number>>
}) {
  useEffect(() => {
    // Move the focus cursor to a node and recenter it. Traversal moves focus
    // only; the panel (selectedNodeId) follows along just when it's already
    // open (sticky-follow), so you can keyboard-traverse with the panel closed.
    // The +110 offset leaves room for the panel slide-over — dropped when the
    // panel is closed so the node lands dead-center.
    const moveFocus = (node: Node) => {
      const panelOpen = selectedNodeId !== null
      setFocusNodeId(node.id)
      if (panelOpen) setSelectedNodeId(node.id)
      markInteraction(450)
      if (node.position?.x !== undefined && node.position?.y !== undefined) {
        const xOffset = panelOpen ? 110 : 0
        setCenter(node.position.x + xOffset, node.position.y, {
          duration: 400,
          zoom: Math.max(getZoom(), 0.8),
        })
      }
    }

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

      // Escape is owned by the capture-phase handler below (progressive
      // dismiss: filter → panel → focus cursor), so it can stopPropagation
      // before ReactFlow sees it. Nothing to do here.

      // ? opens the keyboard-shortcut help overlay.
      if (e.key === '?') {
        e.preventDefault()
        setHelpOpen(true)
        return
      }

      // "/" (and Cmd/Ctrl+"/", which also reports e.key === '/') opens the
      // node search — the primary way to search the map.
      if (e.key === '/') {
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
      // M = toggle In Motion lens
      if (e.key === 'm' || e.key === 'M') {
        e.preventDefault()
        setActiveFilter((f) => f === 'in_motion' ? null : 'in_motion')
        return
      }
      // B = toggle Blocked lens
      if (e.key === 'b' || e.key === 'B') {
        e.preventDefault()
        setActiveFilter((f) => f === 'blocked' ? null : 'blocked')
        return
      }
      // D = toggle Done lens (highlight completed work)
      if (e.key === 'd' || e.key === 'D') {
        e.preventDefault()
        setActiveFilter((f) => f === 'done' ? null : 'done')
        return
      }
      // R = toggle Decisions (reasoning) lens
      if (e.key === 'r' || e.key === 'R') {
        e.preventDefault()
        setActiveFilter((f) => f === 'decisions' ? null : 'decisions')
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
            moveFocus(missionsSorted[0])
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

        moveFocus(nextMission)
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
          if (nextNode) moveFocus(nextNode)
        }
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [effectiveFocusNodeId, displayNodes, nodeById, missionsSorted, setCenter, getZoom, markInteraction, fitView, zoomIn, zoomOut, helpOpen, searchOpen, filterResult, selectedNodeId])

  // Capture-phase handler for keys that ReactFlow may swallow.
  // Fires before any element can stopPropagation.
  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (helpOpen || searchOpen) return

      if (e.key === 'Escape') {
        // Progressive dismiss, one concern per press:
        //   1. an active filter, then
        //   2. the open panel (but keep the focus cursor so you can keep
        //      traversing with the panel closed), then
        //   3. the focus cursor itself.
        if (activeFilter) {
          e.stopPropagation()
          setActiveFilter(null)
          return
        }
        if (selectedNodeId) {
          e.stopPropagation()
          setSelectedNodeId(null)
          return
        }
        if (focusNodeId) {
          e.stopPropagation()
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
}
