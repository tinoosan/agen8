import { describe, it, expect, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { ResizeHandle } from './ResizeHandle'

describe('ResizeHandle', () => {
  it('reports the horizontal delta between pointer moves during a drag', () => {
    const onResize = vi.fn()
    render(<ResizeHandle onResize={onResize} />)
    const handle = screen.getByRole('separator', { name: /resize panel/i })

    handle.dispatchEvent(new PointerEvent('pointerdown', { clientX: 500, bubbles: true }))
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 470 }))
    expect(onResize).toHaveBeenLastCalledWith(-30) // moved left 30px

    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 490 }))
    expect(onResize).toHaveBeenLastCalledWith(20) // delta since last move, right 20px

    window.dispatchEvent(new PointerEvent('pointerup', {}))
    onResize.mockClear()
    // After release, further moves are ignored.
    window.dispatchEvent(new PointerEvent('pointermove', { clientX: 600 }))
    expect(onResize).not.toHaveBeenCalled()
  })

  it('exposes a vertical separator role for accessibility', () => {
    render(<ResizeHandle onResize={() => {}} aria-label="Resize artifact viewer" />)
    const handle = screen.getByRole('separator', { name: 'Resize artifact viewer' })
    expect(handle.getAttribute('aria-orientation')).toBe('vertical')
  })
})
