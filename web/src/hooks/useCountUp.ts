import { useEffect, useRef, useState } from 'react'

/**
 * Animates a number from 0 to the target value over `duration` ms.
 * Uses requestAnimationFrame for smooth 60fps animation.
 * Returns the current display value as a string (formatted via the
 * optional `format` callback, defaults to Math.round).
 */
export function useCountUp(
  target: number,
  opts?: { duration?: number; format?: (n: number) => string },
): string {
  const duration = opts?.duration ?? 600
  const format = opts?.format ?? ((n: number) => String(Math.round(n)))

  const [display, setDisplay] = useState(() => format(0))
  const prevTarget = useRef(0)
  const rafRef = useRef<number>(0)

  useEffect(() => {
    const from = prevTarget.current
    prevTarget.current = target

    // Skip animation: same value, or reduced motion preference
    const skipAnimation =
      target === from ||
      window.matchMedia('(prefers-reduced-motion: reduce)').matches

    if (skipAnimation) {
      // Use rAF to avoid synchronous setState in effect body
      rafRef.current = requestAnimationFrame(() => {
        setDisplay(format(target))
      })
      return () => cancelAnimationFrame(rafRef.current)
    }

    const start = performance.now()

    function tick(now: number) {
      const elapsed = now - start
      const progress = Math.min(elapsed / duration, 1)
      // ease-out cubic
      const eased = 1 - Math.pow(1 - progress, 3)
      const current = from + (target - from) * eased
      setDisplay(format(current))

      if (progress < 1) {
        rafRef.current = requestAnimationFrame(tick)
      }
    }

    rafRef.current = requestAnimationFrame(tick)
    return () => cancelAnimationFrame(rafRef.current)
  }, [target, duration, format])

  return display
}
