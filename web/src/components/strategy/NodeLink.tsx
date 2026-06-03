import type { ReactNode } from 'react'
import { useNodeNavigate } from './useNodeNavigate'
import { usePanelNavigation } from './PanelNavigationContext'

interface Props {
  nodeId: string
  children: ReactNode
  className?: string
}

/**
 * Clickable inline element that navigates to a node.
 *
 * When inside a PanelNavigationContext (e.g., space context panel),
 * clicks push onto the local panel stack for drill-down navigation.
 * Otherwise, falls back to setting pendingFocusNodeId to navigate
 * on the strategy map.
 */
export function NodeLink({ nodeId, children, className }: Props) {
  const panelNav = usePanelNavigation()
  const mapNav = useNodeNavigate()

  const handleClick = () => {
    if (panelNav) {
      panelNav(nodeId)
    } else {
      mapNav(nodeId)
    }
  }

  return (
    <button
      type="button"
      onClick={handleClick}
      className={className ?? 'text-left w-full'}
      style={{ cursor: 'pointer' }}
    >
      {children}
    </button>
  )
}
