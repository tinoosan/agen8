import { useEffect, useMemo, useReducer, useRef } from 'react'
import type { Edge, Node } from '@xyflow/react'
import type { LeafDisplayMode, LeafDisplayPhase, StrategyMapDisplayMode } from './strategyMapRenderState'

const MISSION_KR_ENTER_FULL_ZOOM = 0.2
const MISSION_KR_EXIT_FULL_ZOOM = 0.3
const LEAF_ENTER_FULL_ZOOM = 0.75
const LEAF_EXIT_FULL_ZOOM = 0.3
const LEAF_ENTER_MS = 140
const LEAF_EXIT_MS = 120

export function getNextDisplayMode(
  zoom: number,
  previous: StrategyMapDisplayMode,
): StrategyMapDisplayMode {
  let missionKR = previous.missionKR
  if (previous.missionKR === 'orbit') {
    missionKR = zoom >= MISSION_KR_ENTER_FULL_ZOOM ? 'full' : 'orbit'
  } else {
    missionKR = zoom <= MISSION_KR_EXIT_FULL_ZOOM ? 'orbit' : 'full'
  }

  let leaf = previous.leaf
  if (previous.leaf === 'dot') {
    leaf = zoom >= LEAF_ENTER_FULL_ZOOM ? 'full' : 'dot'
  } else {
    leaf = zoom <= LEAF_EXIT_FULL_ZOOM ? 'dot' : 'full'
  }

  if (missionKR === previous.missionKR && leaf === previous.leaf) {
    return previous
  }

  return { missionKR, leaf }
}

export interface StrategyGraphSnapshot {
  nodes: Node[]
  edges: Edge[]
  isLoading: boolean
}

interface DeferredStrategyGraphState {
  displayed: StrategyGraphSnapshot
  queued: StrategyGraphSnapshot | null
}

function deferredStrategyGraphReducer(
  state: DeferredStrategyGraphState,
  action: { incoming: StrategyGraphSnapshot; isInteracting: boolean },
): DeferredStrategyGraphState {
  if (action.isInteracting) {
    return {
      displayed: state.displayed,
      queued: action.incoming,
    }
  }

  return {
    displayed: state.queued ?? action.incoming,
    queued: null,
  }
}

export function useDeferredStrategyGraph(
  incoming: StrategyGraphSnapshot,
  isInteracting: boolean,
) {
  const [state, dispatch] = useReducer(deferredStrategyGraphReducer, {
    displayed: incoming,
    queued: null,
  })

  useEffect(() => {
    dispatch({ incoming, isInteracting })
  }, [incoming, isInteracting])

  return state.displayed
}

export function useLeafDisplayPhase(leafMode: LeafDisplayMode): LeafDisplayPhase {
  const [phase, dispatchPhase] = useReducer(
    (
      _: LeafDisplayPhase,
      action: { type: 'begin'; mode: LeafDisplayMode } | { type: 'settle'; mode: LeafDisplayMode },
    ): LeafDisplayPhase => {
      if (action.type === 'begin') {
        return action.mode === 'full' ? 'toFull' : 'toDot'
      }
      if (action.mode === 'full') {
        return 'full'
      }
      return 'dot'
    },
    leafMode === 'full' ? 'full' : 'dot',
  )
  const previousModeRef = useRef(leafMode)
  const timerRef = useRef<number | null>(null)

  useEffect(() => {
    return () => {
      if (timerRef.current != null) {
        window.clearTimeout(timerRef.current)
      }
    }
  }, [])

  useEffect(() => {
    if (previousModeRef.current === leafMode) {
      return
    }
    previousModeRef.current = leafMode

    if (timerRef.current != null) {
      window.clearTimeout(timerRef.current)
      timerRef.current = null
    }

    if (leafMode === 'full') {
      dispatchPhase({ type: 'begin', mode: 'full' })
      timerRef.current = window.setTimeout(() => {
        dispatchPhase({ type: 'settle', mode: 'full' })
        timerRef.current = null
      }, LEAF_ENTER_MS)
      return
    }

    dispatchPhase({ type: 'begin', mode: 'dot' })
    timerRef.current = window.setTimeout(() => {
      dispatchPhase({ type: 'settle', mode: 'dot' })
      timerRef.current = null
    }, LEAF_EXIT_MS)
  }, [leafMode])

  return phase
}

export function useStableSelectionNode(
  selectedNodeId: string | null,
  nodes: Node[],
) {
  return useMemo(() => {
    if (!selectedNodeId) return null
    return nodes.find(node => node.id === selectedNodeId) ?? null
  }, [selectedNodeId, nodes])
}
