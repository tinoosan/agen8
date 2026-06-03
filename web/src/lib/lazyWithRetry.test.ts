import { describe, expect, it } from 'vitest'
import { clearLazyRetryMarker, isRetryableDynamicImportError, shouldReloadAfterImportError } from './lazyWithRetry'

class MemoryStorage implements Storage {
  private data = new Map<string, string>()

  get length(): number {
    return this.data.size
  }

  clear(): void {
    this.data.clear()
  }

  getItem(key: string): string | null {
    return this.data.has(key) ? this.data.get(key)! : null
  }

  key(index: number): string | null {
    return Array.from(this.data.keys())[index] ?? null
  }

  removeItem(key: string): void {
    this.data.delete(key)
  }

  setItem(key: string, value: string): void {
    this.data.set(key, value)
  }
}

describe('lazyWithRetry helpers', () => {
  it('detects retryable dynamic import errors', () => {
    expect(isRetryableDynamicImportError(new Error('Failed to fetch dynamically imported module'))).toBe(true)
    expect(isRetryableDynamicImportError(new Error('GET /node_modules/.vite/deps/yaml.js net::ERR_ABORTED 504 (Outdated Optimize Dep)'))).toBe(true)
    expect(isRetryableDynamicImportError(new Error('something else'))).toBe(false)
  })

  it('retries at most once per import key', () => {
    const storage = new MemoryStorage()
    const importKey = 'pages/SpaceFocus'
    const err = new Error('Failed to fetch dynamically imported module')

    expect(shouldReloadAfterImportError(importKey, err, storage)).toBe(true)
    expect(shouldReloadAfterImportError(importKey, err, storage)).toBe(false)
    expect(storage.getItem('agen8:lazy-retry:' + importKey)).toBeNull()
  })

  it('clears retry marker after successful import', () => {
    const storage = new MemoryStorage()
    const importKey = 'pages/SpaceFocus'
    storage.setItem('agen8:lazy-retry:' + importKey, '1')
    clearLazyRetryMarker(importKey, storage)
    expect(storage.getItem('agen8:lazy-retry:' + importKey)).toBeNull()
  })
})

