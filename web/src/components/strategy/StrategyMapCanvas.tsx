import { useEffect, useRef } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import { ReactFlow, type Node, type Edge } from '@xyflow/react'
import { reactFlowNodeTypes, reactFlowEdgeTypes } from './registry'
import { getNextDisplayMode } from './strategyMapPerformance'
import type { StrategyMapDisplayMode } from './strategyMapRenderState'
import { saveViewport, type SavedViewport } from './useStrategyMapViewport'

/**
 * The ReactFlow canvas itself plus its pan/zoom bookkeeping. Owns the
 * viewport-tracking refs and the move handlers that:
 *   - mark interaction / zooming so the deferred-graph + display-mode logic
 *     can settle after a gesture, and
 *   - persist the resting viewport so navigating away and back keeps the
 *     user's place on large maps.
 * Selection/click behavior is owned by the parent and passed in as handlers.
 */
export function StrategyMapCanvas({
  nodes,
  edges,
  onNodeClick,
  onNodeDoubleClick,
  onEdgeClick,
  onPaneClick,
  markInteraction,
  markZooming,
  setDisplayMode,
  projectId,
  initialViewport,
}: {
  nodes: Node[]
  edges: Edge[]
  onNodeClick: (event: React.MouseEvent, node: Node) => void
  onNodeDoubleClick: (event: React.MouseEvent, node: Node) => void
  onEdgeClick: (event: React.MouseEvent, edge: Edge) => void
  onPaneClick: () => void
  markInteraction: (settleDelay?: number) => void
  markZooming: (settleDelay?: number) => void
  setDisplayMode: Dispatch<SetStateAction<StrategyMapDisplayMode>>
  projectId: string
  initialViewport: SavedViewport | null
}) {
  const ZOOM_EPSILON = 0.0005
  const displayModeReleaseTimerRef = useRef<number | null>(null)
  const pendingZoomRef = useRef(1)
  const moveStartZoomRef = useRef(1)
  const lastViewportZoomRef = useRef(1)

  useEffect(() => {
    return () => {
      if (displayModeReleaseTimerRef.current != null) {
        window.clearTimeout(displayModeReleaseTimerRef.current)
      }
    }
  }, [])

  return (
    <ReactFlow
      nodes={nodes}
      edges={edges}
      nodeTypes={reactFlowNodeTypes}
      edgeTypes={reactFlowEdgeTypes}
      onNodeClick={onNodeClick}
      onNodeDoubleClick={onNodeDoubleClick}
      onEdgeClick={onEdgeClick}
      onPaneClick={onPaneClick}
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
      // — no flash. The useEffect in the parent still calls setViewport once
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
  )
}
