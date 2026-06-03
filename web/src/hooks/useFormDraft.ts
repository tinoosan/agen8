import { useState, useEffect, useRef, useCallback } from 'react'

const DRAFT_PREFIX = 'oa-draft-'
const DEBOUNCE_MS = 1000

/**
 * Auto-save form state to localStorage with debounced writes.
 *
 * Key format: `oa-draft-<key>` (e.g. `oa-draft-action-123`).
 * Restores saved draft on mount and when `key` changes (e.g. editing a
 * different task without unmounting). Call `clear()` on successful submit.
 *
 * All localStorage access is wrapped in try/catch for private browsing safety.
 * If a debounced write fails, `hasDraft` is reverted to `false` so the UI
 * never claims a draft exists when nothing was persisted.
 */
export function useFormDraft<T extends Record<string, unknown>>(
  key: string,
  initialValues: T,
): {
  values: T
  hasDraft: boolean
  update: (partial: Partial<T>) => void
  clear: () => void
} {
  const storageKey = DRAFT_PREFIX + key

  const initialRef = useRef(initialValues)

  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null)

  // Keep initialValues ref in sync for clear() and key-change resets.
  // Must be in an effect (not render) to satisfy react-hooks/refs rule.
  useEffect(() => {
    initialRef.current = initialValues
  })

  const [values, setValues] = useState<T>(() => {
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) {
        const parsed = JSON.parse(raw)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          return { ...initialValues, ...parsed } as T
        }
      }
    } catch {
      // Corrupted JSON or localStorage blocked — fall back to initial values
    }
    return initialValues
  })

  const [hasDraft, setHasDraft] = useState(() => {
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) {
        JSON.parse(raw) // validate it's parseable
        return true
      }
    } catch {
      // Corrupted or inaccessible — no valid draft
    }
    return false
  })

  // Re-initialize when key changes (e.g. switching to a different task).
  const isFirstRun = useRef(true)
  useEffect(() => {
    if (isFirstRun.current) {
      isFirstRun.current = false
      return
    }
    // Cancel any pending save for the old key.
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    // Read draft for the new key.
    try {
      const raw = localStorage.getItem(storageKey)
      if (raw) {
        const parsed = JSON.parse(raw)
        if (parsed && typeof parsed === 'object' && !Array.isArray(parsed)) {
          setValues({ ...initialRef.current, ...parsed } as T)
          setHasDraft(true)
          return
        }
      }
    } catch {
      // Corrupted or inaccessible
    }
    setValues(initialRef.current)
    setHasDraft(false)
  }, [storageKey])

  // Ref to track the latest values for the debounced save
  const valuesRef = useRef(values)
  useEffect(() => {
    valuesRef.current = values
  }, [values])

  const scheduleSave = useCallback(() => {
    if (timerRef.current) clearTimeout(timerRef.current)
    timerRef.current = setTimeout(() => {
      try {
        localStorage.setItem(storageKey, JSON.stringify(valuesRef.current))
      } catch {
        // localStorage blocked or full — revert hasDraft so the UI does not
        // claim a draft exists when nothing was persisted.
        setHasDraft(false)
      }
    }, DEBOUNCE_MS)
  }, [storageKey])

  const update = useCallback(
    (partial: Partial<T>) => {
      setValues(prev => {
        const next = { ...prev, ...partial }
        return next
      })
      setHasDraft(true)
      scheduleSave()
    },
    [scheduleSave],
  )

  const clear = useCallback(() => {
    if (timerRef.current) {
      clearTimeout(timerRef.current)
      timerRef.current = null
    }
    try {
      localStorage.removeItem(storageKey)
    } catch {
      // Silently degrade — removal failure is not critical
    }
    setValues(initialRef.current)
    setHasDraft(false)
  }, [storageKey])

  // Cleanup timer on unmount
  useEffect(() => {
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current)
    }
  }, [])

  return { values, hasDraft, update, clear }
}
