import { useEffect, useRef } from 'react'
import type { Dispatch, SetStateAction } from 'react'
import type { Node } from '@xyflow/react'
import { useStrategyMapStore } from './strategyMapStore'
import type { StrategyMapDisplayMode, LeafDisplayPhase } from './strategyMapRenderState'
import type { FilterPreset } from './strategyMapFilters'
import type { SetCenterFn } from './strategyMapControls'

/**
 * Two concerns that both revolve around focus:
 *
 *  1. Deep-link focus — read a ?focus= query param on mount (and watch the
 *     store's pendingFocusNodeId set by the command palette), then select +
 *     center that node once it appears in the graph.
 *  2. Store mirroring — push React-owned render state into the Zustand store
 *     so node/edge components can subscribe to individual fields via selectors
 *     instead of re-rendering on every interaction flip.
 */
export function useStrategyMapFocusEffects({
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
}: {
  displayNodes: Node[]
  setCenter: SetCenterFn
  setFocusNodeId: Dispatch<SetStateAction<string | null>>
  setSelectedNodeId: Dispatch<SetStateAction<string | null>>
  displayMode: StrategyMapDisplayMode
  leafPhase: LeafDisplayPhase
  isInteracting: boolean
  isZooming: boolean
  isDense: boolean
  effectiveFocusNodeId: string | null
  selectedNodeId: string | null
  clusterNodeIds: ReadonlySet<string> | null
  directEdgeIds: ReadonlySet<string> | null
  clusterEdgeIds: ReadonlySet<string> | null
  activeFilter: FilterPreset | null
}) {
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
  }, [displayNodes, setCenter, storePendingFocus, setFocusNodeId, setSelectedNodeId])

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
}
