import type {
  FitViewOptions,
  SetCenterOptions,
  Viewport,
  ViewportHelperFunctionOptions,
} from '@xyflow/react'

/* ── React Flow control-function type aliases ─────────────────
   The strategy-map hooks receive a handful of imperative React
   Flow controls (from useReactFlow) as arguments. These aliases
   keep the parameter signatures identical to the real ones so
   passing them across the hook boundary type-checks cleanly. We
   widen the return to void since the hooks ignore the promises. */

export type FitViewFn = (options?: FitViewOptions) => void
export type SetViewportFn = (viewport: Viewport, options?: ViewportHelperFunctionOptions) => void
export type SetCenterFn = (x: number, y: number, options?: SetCenterOptions) => void
export type GetZoomFn = () => number
export type ZoomFn = (options?: ViewportHelperFunctionOptions) => void
