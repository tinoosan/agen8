import { useCallback } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { FitViewFn, GetZoomFn, SetCenterFn } from './strategyMapControls'

/**
 * Mouse/touch interaction handlers for the canvas. Traversal (single tap on a
 * node, edge click) moves the focus *cursor* and recenters — it does NOT open
 * the detail panel on its own. The panel (selectedNodeId) only follows along
 * when it is already open (sticky-follow), so you can traverse the graph with
 * the panel closed. Opening the panel is a deliberate gesture: tapping the
 * already-focused node (or Enter/Space, handled in the keyboard hook).
 * Pane clicks clear selection; double-clicking empty canvas fits the map.
 * Pure callbacks over the parent's selection state — no state of its own.
 *
 * `selectedNodeId` is threaded in only to answer "is the panel open?" — it is
 * non-null exactly when the panel is showing.
 */
export function useStrategyMapNodeHandlers({
  effectiveFocusNodeId,
  selectedNodeId,
  nodeById,
  setCenter,
  getZoom,
  fitView,
  markInteraction,
  setFocusNodeId,
  setSelectedNodeId,
}: {
  effectiveFocusNodeId: string | null
  selectedNodeId: string | null
  nodeById: Map<string, Node>
  setCenter: SetCenterFn
  getZoom: GetZoomFn
  fitView: FitViewFn
  markInteraction: (settleDelay?: number) => void
  setFocusNodeId: Dispatch<SetStateAction<string | null>>
  setSelectedNodeId: Dispatch<SetStateAction<string | null>>
}) {
  const handleEdgeClick = useCallback((_: React.MouseEvent, edge: Edge) => {
    // Navigate to the other end of the edge relative to the focused node.
    // If no node is focused, default to the target (child) end.
    const goToId = effectiveFocusNodeId === edge.target ? edge.source
      : effectiveFocusNodeId === edge.source ? edge.target
      : edge.target
    const goToNode = nodeById.get(goToId)
    if (goToNode) {
      const panelOpen = selectedNodeId !== null
      setFocusNodeId(goToNode.id)
      // Sticky-follow: only carry the panel along if it's already open. With
      // the panel closed, hopping along an edge keeps it closed.
      if (panelOpen) setSelectedNodeId(goToNode.id)
      markInteraction(450)
      if (goToNode.position?.x !== undefined && goToNode.position?.y !== undefined) {
        // +110 leaves room for the panel's slide-over; with the panel closed
        // the node should land dead-center, so drop the offset.
        const xOffset = panelOpen ? 110 : 0
        setCenter(goToNode.position.x + xOffset, goToNode.position.y, {
          duration: 400,
          zoom: Math.max(getZoom(), 0.8)
        })
      }
    }
  }, [effectiveFocusNodeId, selectedNodeId, nodeById, setCenter, getZoom, markInteraction, setFocusNodeId, setSelectedNodeId])

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    const alreadyFocused = node.id === effectiveFocusNodeId
    const panelOpen = selectedNodeId !== null
    // Always move the cursor to the tapped node.
    setFocusNodeId(node.id)
    // Open the panel only on a deliberate gesture — a second tap on the node
    // that's already focused — or keep it following if it's already open.
    if (alreadyFocused || panelOpen) {
      setSelectedNodeId(node.id)
    }
  }, [effectiveFocusNodeId, selectedNodeId, setSelectedNodeId, setFocusNodeId])

  const handleNodeDoubleClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Cinematic push to closely inspect a node. The single-click handler fires
    // first and focuses the node; this just zooms in. +110 makes room for the
    // panel slide-over when it's open, otherwise center the node fully.
    markInteraction(450)
    const xOffset = selectedNodeId !== null ? 110 : 0
    setCenter((node.position?.x ?? 0) + xOffset, (node.position?.y ?? 0), { zoom: 1.6, duration: 400 })
  }, [selectedNodeId, setCenter, markInteraction])

  const handlePaneClick = useCallback(() => {
    setSelectedNodeId(null)
    setFocusNodeId(null)
  }, [setSelectedNodeId, setFocusNodeId])

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
  }, [fitView, markInteraction, setSelectedNodeId, setFocusNodeId])

  return {
    handleEdgeClick,
    handleNodeClick,
    handleNodeDoubleClick,
    handlePaneClick,
    handlePaneDoubleClick,
  }
}
