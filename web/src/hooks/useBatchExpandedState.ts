import { useCallback, useState } from 'react'

/**
 * Persisted expansion state for a batch card, keyed on the batch's first
 * event id. Writes are synchronous (toggles are human-paced clicks — no
 * debounce needed). On `QuotaExceededError` we nuke all keys under the
 * `oa-batch-expanded:` prefix and retry once — acceptable because the
 * default state is collapsed, so losing stored preferences only means
 * users re-collapse anything they happened to expand.
 *
 * No SSR guards — Vite SPA, `window` always exists. Precedent:
 * ThoughtBubble.tsx uses the same unguarded pattern.
 *
 * Storage format: `'0'` / `'1'` string literals. No JSON, no serialization
 * overhead. Human-readable in DevTools.
 */

const KEY_PREFIX = 'oa-batch-expanded:'

function storageKey(firstEventId: string): string {
  return `${KEY_PREFIX}${firstEventId}`
}

/** Read a key. Returns null for missing, corrupt, or inaccessible storage. */
function readStored(firstEventId: string): boolean | null {
  try {
    const raw = localStorage.getItem(storageKey(firstEventId))
    if (raw === '1') return true
    if (raw === '0') return false
    return null
  } catch {
    // Private browsing, storage disabled, etc.
    return null
  }
}

/**
 * Nuclear sweep: delete every key with our prefix. Called when a write
 * fails due to quota exhaustion. Iterates via a key snapshot because
 * localStorage mutation during iteration is undefined.
 *
 * Exported for tests so they can exercise the prune in isolation from
 * the mocked-setItem retry path.
 */
export function __pruneAllBatchExpandedKeysForTests(): void {
  pruneAllBatchExpandedKeys()
}

function pruneAllBatchExpandedKeys(): void {
  try {
    const victims: string[] = []
    for (let i = 0; i < localStorage.length; i++) {
      const key = localStorage.key(i)
      if (key && key.startsWith(KEY_PREFIX)) victims.push(key)
    }
    for (const key of victims) {
      try { localStorage.removeItem(key) } catch { /* ignore individual failures */ }
    }
  } catch {
    // Even the scan failed — nothing we can do, the setter will fail again.
  }
}

/** Write a key. Handles quota exhaustion via nuclear sweep + retry once. */
function writeStored(firstEventId: string, expanded: boolean): void {
  const key = storageKey(firstEventId)
  const value = expanded ? '1' : '0'
  try {
    localStorage.setItem(key, value)
  } catch (err) {
    // DOMException name is 'QuotaExceededError' in modern browsers; older
    // Safari uses 'QUOTA_EXCEEDED_ERR'. Rather than sniffing names, retry
    // once after a sweep and swallow any further failure — losing
    // persistence is the documented fallback.
    if (err instanceof Error) {
      pruneAllBatchExpandedKeys()
      try { localStorage.setItem(key, value) } catch { /* give up silently */ }
    }
  }
}

/**
 * Persistent expansion toggle for a batch card.
 *
 *   const [expanded, setExpanded] = useBatchExpandedState(events[0].id)
 *
 * Returns `[expanded, setExpanded]` like `useState<boolean>`. Initial
 * read tries storage once; subsequent toggles write synchronously.
 * Defaults to collapsed when no entry exists.
 */
export function useBatchExpandedState(
  firstEventId: string,
  defaultExpanded: boolean = false,
): [boolean, (next: boolean) => void] {
  // Initial read is in the useState lazy initializer so it only runs
  // once per mount, not on every render.
  const [expanded, setExpandedState] = useState<boolean>(() => {
    const stored = readStored(firstEventId)
    return stored ?? defaultExpanded
  })

  const setExpanded = useCallback((next: boolean) => {
    setExpandedState(next)
    writeStored(firstEventId, next)
  }, [firstEventId])

  return [expanded, setExpanded]
}
