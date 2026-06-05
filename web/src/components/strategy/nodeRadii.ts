/**
 * Collision radii for each strategy-map node type. Kept in a zero-dependency
 * module so the force-layout Web Worker (which must not import React) and
 * `registry.ts` (which imports the React components) can both reference the
 * same source of truth without drift.
 */
export const NODE_RADIUS: Record<string, number> = {
  mission: 120,
  keyResult: 100,
  decision: 70,
  task: 70,
}

export const DEFAULT_RADIUS = 12
