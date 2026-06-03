import { useState, useCallback } from 'react'

/**
 * Hook for resizable summary zones in detail panels.
 * Persists the height to localStorage so it survives across sessions.
 */
export function useResizableSummary(
  storageKey: string,
  { min = 80, max = 480, defaultHeight = 200 }: { min?: number; max?: number; defaultHeight?: number } = {},
) {
  const [height, setHeight] = useState(() => {
    try {
      const stored = localStorage.getItem(storageKey)
      if (stored) {
        const n = parseInt(stored, 10)
        if (n >= min && n <= max) return n
      }
    } catch { /* ignore */ }
    return defaultHeight
  })

  const onResizeStart = useCallback((e: React.MouseEvent) => {
    e.preventDefault()
    const startY = e.clientY
    const startH = height

    const onMove = (ev: MouseEvent) => {
      const next = Math.max(min, Math.min(max, startH + ev.clientY - startY))
      setHeight(next)
      try { localStorage.setItem(storageKey, String(next)) } catch { /* ignore */ }
    }
    const onUp = () => {
      window.removeEventListener('mousemove', onMove)
      window.removeEventListener('mouseup', onUp)
    }
    window.addEventListener('mousemove', onMove)
    window.addEventListener('mouseup', onUp)
  }, [height, min, max, storageKey])

  return { height, onResizeStart }
}
