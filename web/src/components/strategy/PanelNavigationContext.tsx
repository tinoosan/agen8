import { createContext, useContext } from 'react'

/**
 * When a panel is rendered inside a navigation stack (e.g., a context
 * panel with its own drill-down history), NodeLink uses this context to
 * push onto the local stack instead of navigating the strategy map.
 *
 * When absent (strategy map DetailPanel), NodeLink falls back to
 * setting pendingFocusNodeId in the zustand store.
 */
export const PanelNavigationContext = createContext<((nodeId: string) => void) | null>(null)

export function usePanelNavigation(): ((nodeId: string) => void) | null {
  return useContext(PanelNavigationContext)
}
