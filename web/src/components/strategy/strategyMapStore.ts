import { create } from 'zustand'
import type { LeafDisplayPhase, StrategyMapDisplayMode } from './strategyMapRenderState'
import type { FilterPreset } from './strategyMapFilters'

/**
 * Strategy-map render state, backed by Zustand so node/edge components can
 * subscribe to *individual* fields via selectors instead of reading a single
 * monolithic context object.
 *
 * Why this exists: the previous `StrategyMapRenderContext` packed ten pieces
 * of state into one provider value. Any change to any field (most commonly
 * `isInteracting` flipping every frame of a pan gesture) triggered every
 * consumer — i.e. every node and every edge — to re-render, bypassing
 * `React.memo` entirely. On a 100+ node map that's hundreds of components
 * committing per frame during a pan.
 *
 * With this store, a component calls e.g. `useStrategyMapStore(s => s.leafPhase)`
 * and only re-renders when *that specific field* changes. An `isInteracting`
 * flip now invalidates only the ~5 subscriptions that actually read it.
 *
 * The store is a pure mirror — StrategyMap.tsx owns the React state and
 * pushes it into the store via `useEffect` on every change. No actions live
 * on the store itself; setState from the component is sufficient and keeps
 * the single-source-of-truth in React.
 */
export interface StrategyMapStoreState {
  displayMode: StrategyMapDisplayMode
  leafPhase: LeafDisplayPhase
  isInteracting: boolean
  isZooming: boolean
  isDense: boolean
  focusNodeId: string | null
  selectedNodeId: string | null
  clusterNodeIds: ReadonlySet<string> | null
  directEdgeIds: ReadonlySet<string> | null
  clusterEdgeIds: ReadonlySet<string> | null
  /** Set by external surfaces (deep-link/search entrypoints) to request focus on a node. */
  pendingFocusNodeId: string | null
  /** Active smart filter preset (in_motion/blocked/done/decisions). */
  activeFilter: FilterPreset | null
}

export const useStrategyMapStore = create<StrategyMapStoreState>(() => ({
  displayMode: { missionKR: 'full', leaf: 'full' },
  leafPhase: 'full',
  isInteracting: false,
  isZooming: false,
  isDense: false,
  focusNodeId: null,
  selectedNodeId: null,
  clusterNodeIds: null,
  directEdgeIds: null,
  clusterEdgeIds: null,
  pendingFocusNodeId: null,
  activeFilter: null,
}))
