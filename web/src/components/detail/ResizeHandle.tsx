import { useCallback, useRef } from 'react'

interface ResizeHandleProps {
  /** Called with the horizontal pointer delta (px) since the last move event.
   *  Positive = pointer moved right, negative = left. */
  onResize: (deltaX: number) => void
  'aria-label'?: string
}

/**
 * A thin vertical drag handle for resizing the element to its right (or left,
 * depending on how the consumer applies the delta). Uses pointer capture so the
 * drag survives the cursor leaving the handle, and sets a body cursor + disables
 * text selection for the duration.
 */
export function ResizeHandle({ onResize, 'aria-label': ariaLabel = 'Resize panel' }: ResizeHandleProps) {
  const lastX = useRef<number | null>(null)

  const handlePointerMove = useCallback(
    (e: PointerEvent) => {
      if (lastX.current === null) return
      const delta = e.clientX - lastX.current
      lastX.current = e.clientX
      onResize(delta)
    },
    [onResize],
  )

  const endDrag = useCallback(() => {
    lastX.current = null
    document.body.style.removeProperty('cursor')
    document.body.style.removeProperty('user-select')
    window.removeEventListener('pointermove', handlePointerMove)
    window.removeEventListener('pointerup', endDrag)
  }, [handlePointerMove])

  const handlePointerDown = useCallback(
    (e: React.PointerEvent) => {
      e.preventDefault()
      lastX.current = e.clientX
      document.body.style.cursor = 'col-resize'
      document.body.style.userSelect = 'none'
      window.addEventListener('pointermove', handlePointerMove)
      window.addEventListener('pointerup', endDrag)
    },
    [handlePointerMove, endDrag],
  )

  return (
    <div
      role="separator"
      aria-orientation="vertical"
      aria-label={ariaLabel}
      onPointerDown={handlePointerDown}
      className="group relative w-1 shrink-0 cursor-col-resize select-none touch-none"
    >
      {/* hit area is wider than the visible line */}
      <span className="absolute inset-y-0 -left-1 -right-1" />
      <span className="absolute inset-y-0 left-0 w-px bg-[var(--border)] group-hover:bg-[var(--accent)] transition-colors" />
    </div>
  )
}
