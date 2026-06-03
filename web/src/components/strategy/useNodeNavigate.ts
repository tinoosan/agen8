import { useCallback } from 'react'
import { useStrategyMapStore } from './strategyMapStore'

/**
 * Returns a function that navigates to a node on the strategy map by
 * setting pendingFocusNodeId in the zustand store. The strategy map
 * picks this up and pans/selects the target node.
 *
 * Node ID formats:
 *   Mission / KR  → raw UUID (no prefix)
 *   Task          → task:{id}
 *   Decision      → decision:{id}
 *   Plan          → plan:{id}
 *   OA            → oa:{id}
 *   Escalation    → escalation:{id}
 */
export function useNodeNavigate() {
  return useCallback((nodeId: string) => {
    useStrategyMapStore.setState({ pendingFocusNodeId: nodeId })
  }, [])
}
