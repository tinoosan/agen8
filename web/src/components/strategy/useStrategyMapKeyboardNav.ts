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
}
