import { Maximize2, Minus, Plus } from 'lucide-react'
import { useReactFlow } from '@xyflow/react'

/**
 * On-canvas zoom / fit controls. These are the touch (iPad/tablet) equivalent
 * of the keyboard zoom + fit affordances in useStrategyMapKeyboardNav: those
 * users have no `+` / `-` / `Shift+F` keys, and pinch-zoom alone makes it easy
 * to lose the whole map off-screen. The fit button is the "take me back to the
 * whole map" reset.
 *
 * Durations mirror the keyboard handlers exactly (zoom 200ms, fit 600ms with
 * 0.18 padding) so a tap and a keypress feel identical. The animated viewport
 * change fires ReactFlow's onMove, which is what marks interaction / settles
 * the display mode — so there's no separate markInteraction wiring here.
 */
export function StrategyMapZoomControls() {
  const { zoomIn, zoomOut, fitView } = useReactFlow()

  const buttonStyle: React.CSSProperties = {
    width: 32,
    height: 32,
    borderRadius: 10,
    border: '1px solid var(--border)',
    background: 'var(--bg-panel)',
    color: 'var(--text-3)',
    boxShadow: '0 1px 3px rgba(0,0,0,0.08)',
  }

  return (
    <div className="absolute bottom-5 right-5 z-10 flex flex-col gap-1.5">
      <button
        type="button"
        onClick={() => zoomIn({ duration: 200 })}
        aria-label="Zoom in"
        title="Zoom in (+)"
        className="inline-flex items-center justify-center transition-all duration-200 hover:scale-[1.05] focus:outline-none focus-visible:outline-none"
        style={buttonStyle}
      >
        <Plus size={15} />
      </button>
      <button
        type="button"
        onClick={() => zoomOut({ duration: 200 })}
        aria-label="Zoom out"
        title="Zoom out (-)"
        className="inline-flex items-center justify-center transition-all duration-200 hover:scale-[1.05] focus:outline-none focus-visible:outline-none"
        style={buttonStyle}
      >
        <Minus size={15} />
      </button>
      <button
        type="button"
        onClick={() => fitView({ padding: 0.18, duration: 600 })}
        aria-label="Fit map to view"
        title="Fit map to view (Shift+F)"
        className="inline-flex items-center justify-center transition-all duration-200 hover:scale-[1.05] focus:outline-none focus-visible:outline-none"
        style={buttonStyle}
      >
        <Maximize2 size={14} />
      </button>
    </div>
  )
}
