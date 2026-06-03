export type MissionKRDisplayMode = 'orbit' | 'full'
export type LeafDisplayMode = 'dot' | 'full'
export type LeafDisplayPhase = 'dot' | 'toFull' | 'full' | 'toDot'

export interface StrategyMapDisplayMode {
  missionKR: MissionKRDisplayMode
  leaf: LeafDisplayMode
}

/**
 * Aggregated shape of all strategy-map render state.
 *
 * Kept as a type-only definition (no runtime Context) because every
 * consumer now reads from the Zustand store in `strategyMapStore.ts`
 * via field-level selectors — no monolithic provider value to destructure.
 */
export interface StrategyMapRenderState {
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
}
