import { useState, useCallback, useRef } from 'react'

/**
 * Prevents double-submit and captures server errors (e.g. "already resolved").
 *
 * Usage:
 *   const { isSubmitting, error, guard, clearError } = useConcurrentGuard()
 *   <Button disabled={isSubmitting} onClick={() => guard(async () => { ... })}>
 *
 * While `guard(fn)` is in flight, subsequent calls are silently ignored.
 * On error, the message is captured in `error`. Call `clearError()` to reset.
 */
export function useConcurrentGuard(): {
  isSubmitting: boolean
  error: string | null
  guard: (action: () => Promise<void>) => Promise<void>
  clearError: () => void
} {
  const [isSubmitting, setIsSubmitting] = useState(false)
  const [error, setError] = useState<string | null>(null)
  const inflightRef = useRef(false)

  const guard = useCallback(async (action: () => Promise<void>) => {
    if (inflightRef.current) return

    inflightRef.current = true
    setIsSubmitting(true)
    setError(null)

    try {
      await action()
    } catch (err: unknown) {
      const message = err instanceof Error ? err.message : 'An unexpected error occurred'
      setError(message)
      throw err // Re-throw so callers can distinguish success from failure
    } finally {
      inflightRef.current = false
      setIsSubmitting(false)
    }
  }, [])

  const clearError = useCallback(() => {
    setError(null)
  }, [])

  return { isSubmitting, error, guard, clearError }
}
