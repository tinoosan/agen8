import { render, screen, fireEvent } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { StrategyMapZoomControls } from './StrategyMapZoomControls'

/* These buttons are the touch (iPad/tablet) equivalent of the keyboard
 * `+` / `-` / `Shift+F` zoom + fit affordances — the only way to zoom and
 * recover the whole map when there's no hardware keyboard. The tests pin that
 * the three controls render and drive React Flow's viewport handlers with the
 * same options the keyboard path uses. */
const zoomIn = vi.fn()
const zoomOut = vi.fn()
const fitView = vi.fn()

vi.mock('@xyflow/react', () => ({
  useReactFlow: () => ({ zoomIn, zoomOut, fitView }),
}))

describe('StrategyMapZoomControls', () => {
  it('renders zoom-in, zoom-out, and fit buttons', () => {
    render(<StrategyMapZoomControls />)
    expect(screen.getByLabelText('Zoom in')).toBeInTheDocument()
    expect(screen.getByLabelText('Zoom out')).toBeInTheDocument()
    expect(screen.getByLabelText('Fit map to view')).toBeInTheDocument()
  })

  it('zooms in on click, matching the keyboard duration', () => {
    render(<StrategyMapZoomControls />)
    fireEvent.click(screen.getByLabelText('Zoom in'))
    expect(zoomIn).toHaveBeenCalledWith({ duration: 200 })
  })

  it('zooms out on click, matching the keyboard duration', () => {
    render(<StrategyMapZoomControls />)
    fireEvent.click(screen.getByLabelText('Zoom out'))
    expect(zoomOut).toHaveBeenCalledWith({ duration: 200 })
  })

  it('fits the whole map on click, matching the Shift+F padding/duration', () => {
    render(<StrategyMapZoomControls />)
    fireEvent.click(screen.getByLabelText('Fit map to view'))
    expect(fitView).toHaveBeenCalledWith({ padding: 0.18, duration: 600 })
  })
})
