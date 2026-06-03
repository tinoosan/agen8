import { lazy, type ComponentType, type LazyExoticComponent } from 'react'

const RETRY_MARKER_PREFIX = 'agen8:lazy-retry:'

const RETRYABLE_IMPORT_ERROR_PATTERNS = [
  /Outdated Optimize Dep/i,
  /Failed to fetch dynamically imported module/i,
  /Importing a module script failed/i,
  /ERR_ABORTED\s*504/i,
]

function getErrorMessage(error: unknown): string {
  if (typeof error === 'string') return error
  if (error && typeof error === 'object' && 'message' in error) {
    const message = (error as { message?: unknown }).message
    if (typeof message === 'string') return message
  }
  return ''
}

function getSessionStorage(): Storage | null {
  if (typeof window === 'undefined') return null
  try {
    return window.sessionStorage
  } catch {
    return null
  }
}

function markerKey(importKey: string): string {
  return RETRY_MARKER_PREFIX + importKey
}

export function isRetryableDynamicImportError(error: unknown): boolean {
  const message = getErrorMessage(error)
  if (!message) return false
  return RETRYABLE_IMPORT_ERROR_PATTERNS.some(pattern => pattern.test(message))
}

export function clearLazyRetryMarker(importKey: string, storage: Storage | null = getSessionStorage()): void {
  if (!storage) return
  try {
    storage.removeItem(markerKey(importKey))
  } catch {
    // Best-effort marker cleanup.
  }
}

export function shouldReloadAfterImportError(
  importKey: string,
  error: unknown,
  storage: Storage | null = getSessionStorage(),
): boolean {
  if (!isRetryableDynamicImportError(error) || !storage) return false
  try {
    const key = markerKey(importKey)
    if (storage.getItem(key) === '1') {
      // Already retried once this session for this chunk; fail loud now.
      storage.removeItem(key)
      return false
    }
    storage.setItem(key, '1')
    return true
  } catch {
    return false
  }
}

function reloadWindow(): void {
  if (typeof window === 'undefined') return
  window.location.reload()
}

// lazyWithRetry wraps React.lazy() and handles stale optimize-dep/chunk errors
// by forcing a single hard reload. If the same import fails again after reload,
// we surface the real error to ErrorBoundary.
export function lazyWithRetry<P>(
  importer: () => Promise<{ default: ComponentType<P> }>,
  importKey: string,
): LazyExoticComponent<ComponentType<P>> {
  return lazy(async () => {
    try {
      const mod = await importer()
      clearLazyRetryMarker(importKey)
      return mod
    } catch (error) {
      if (shouldReloadAfterImportError(importKey, error)) {
        reloadWindow()
        return new Promise<never>(() => {})
      }
      throw error
    }
  })
}
