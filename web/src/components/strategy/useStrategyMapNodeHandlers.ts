import { useCallback } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { FitViewFn, GetZoomFn, SetCenterFn } from './strategyMapControls'

/**
 * Mouse interaction handlers for the canvas: clicking a node/edge selects +
 * focuses + cinematically recenters it; double-click pushes in to inspect;
 * pane clicks clear selection; double-clicking empty canvas fits the map.
 * Pure callbacks over the parent's selection state — no state of its own.
 */
export function useStrategyMapNodeHandlers({
  effectiveFocusNodeId,
  nodeById,
  setCenter,
  getZoom,
  fitView,
  markInteraction,
  setFocusNodeId,
  setSelectedNodeId,
}: {
  effectiveFocusNodeId: string | null
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
  }, [effectiveFocusNodeId, nodeById, setCenter, getZoom, markInteraction, setFocusNodeId, setSelectedNodeId])

  const handleNodeClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Assertively lock focus. Clicking a node should never randomly deselect it if clicked twice,
    // which protects the Double-Click Zoom flow from destroying the Slide-Over state!
    setSelectedNodeId(node.id)
    setFocusNodeId(node.id)
  }, [setSelectedNodeId, setFocusNodeId])

  const handleNodeDoubleClick = useCallback((_: React.MouseEvent, node: Node) => {
    // Assertive cinematic push to closely inspect node details
    // The +110 offsets the viewport perfectly allowing the DetailPanel slide-over to coexist!
    markInteraction(450)
    setCenter((node.position?.x ?? 0) + 110, (node.position?.y ?? 0), { zoom: 1.6, duration: 400 })
  }, [setCenter, markInteraction])

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
