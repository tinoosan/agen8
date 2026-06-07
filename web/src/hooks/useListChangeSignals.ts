import { useEffect, useRef, useState } from 'react'

/**
 * Detects which items in a live list have just *changed* between renders, so the
 * UI can play a one-shot "feel alive" acknowledgement on exactly those items.
 * This is the presentation layer on top of the SSE invalidation path
 * (useRealtimeInvalidation): SSE triggers a refetch, React Query swaps the data,
 * and this hook turns that otherwise-silent status swap into a visible flash.
 *
 * Scope is deliberately *changes only* — entrance is left to CSS-on-mount, since
 * a list row keyed by id is a fresh DOM node when it arrives and so its entrance
 * animation already plays only on arrival (never on a steady-state refetch),
 * with no cross-render bookkeeping and no first-frame flicker. What CSS can't see
 * is a value changing inside an existing, reused node — that's what this is for.
 *
 * `signalFn` returns the value whose change counts as a transition — typically a
 * task/member status. When it changes for a key that already existed, that key is
 * flagged for ~`flashMs`, then cleared.
 *
 * Honors prefers-reduced-motion: when set, `changedIds` stays empty so no motion
 * class is ever applied (mirrors the CSS guard and useCountUp). setState is
 * deferred into requestAnimationFrame (same pattern as useCountUp) to avoid
 * synchronous cascading renders from inside the effect.
 */

export interface ChangeSignals {
  /** Keys whose signal value changed vs the previously-shown list (transition). */
  changedIds: Set<string>
}

const EMPTY: Set<string> = new Set()

function prefersReducedMotion(): boolean {
  return (
    typeof window !== 'undefined' &&
    typeof window.matchMedia === 'function' &&
    window.matchMedia('(prefers-reduced-motion: reduce)').matches
  )
}

export function useListChangeSignals<T>(
  items: T[] | undefined,
  keyFn: (item: T) => string,
  signalFn: (item: T) => string,
  opts?: { flashMs?: number },
): ChangeSignals {
  const flashMs = opts?.flashMs ?? 1200

  // Snapshot of key -> last-seen signal. `null` until the first list arrives so
  // we can tell "first load" (announce nothing) apart from "empty list".
  const seen = useRef<Map<string, string> | null>(null)
  // Functions are read through refs so the diff effect depends only on `items`,
  // not on inline closures callers pass. Synced in an effect (declared first so
  // it runs before the diff effect) rather than during render.
  const keyRef = useRef(keyFn)
  const signalRef = useRef(signalFn)
  useEffect(() => {
    keyRef.current = keyFn
    signalRef.current = signalFn
  })

  const [changed, setChanged] = useState<Set<string>>(EMPTY)

  // Pending removal timers, so rapid successive changes to the same key don't
  // strand a flash class on the element.
  const flashTimers = useRef<Map<string, number>>(new Map())
  const rafRef = useRef<number | null>(null)

  useEffect(() => {
    if (!items) return

    const prev = seen.current
    const next = new Map<string, string>()
    const newlyChanged = new Set<string>()

    for (const item of items) {
      const k = keyRef.current(item)
      const sig = signalRef.current(item)
      next.set(k, sig)
      // Only an existing key whose signal moved counts as a transition; brand-new
      // keys are entrances and are handled by CSS-on-mount.
      if (prev !== null && prev.has(k) && prev.get(k) !== sig) newlyChanged.add(k)
    }
    seen.current = next

    // First load or reduced motion: record the baseline, emit no signals.
    if (prev === null || prefersReducedMotion() || newlyChanged.size === 0) return

    // Defer the state update out of the effect body (mirrors useCountUp's rAF
    // pattern) so we don't trigger a synchronous cascading render.
    rafRef.current = requestAnimationFrame(() => {
      setChanged((cur) => {
        const merged = new Set(cur)
        for (const k of newlyChanged) merged.add(k)
        return merged
      })
      for (const k of newlyChanged) {
        const existing = flashTimers.current.get(k)
        if (existing) clearTimeout(existing)
        flashTimers.current.set(
          k,
          window.setTimeout(() => {
            flashTimers.current.delete(k)
            setChanged((cur) => {
              if (!cur.has(k)) return cur
              const n = new Set(cur)
              n.delete(k)
              return n
            })
          }, flashMs),
        )
      }
    })
  }, [items, flashMs])

  // Clear every outstanding timer / frame on unmount.
  useEffect(() => {
    const flashes = flashTimers.current
    return () => {
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current)
      for (const t of flashes.values()) clearTimeout(t)
      flashes.clear()
    }
  }, [])

  return { changedIds: changed }
}
