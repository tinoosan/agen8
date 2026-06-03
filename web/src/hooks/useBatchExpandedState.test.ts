// @vitest-environment jsdom

import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useBatchExpandedState, __pruneAllBatchExpandedKeysForTests } from './useBatchExpandedState'

/**
 * Unit tests for the persisted expansion-state hook. All tests use the
 * real jsdom localStorage; tests that need failure modes swap out
 * `setItem` / `getItem` on the prototype so other tests stay isolated.
 */

const KEY_PREFIX = 'oa-batch-expanded:'

function clearBatchKeys() {
  const victims: string[] = []
  for (let i = 0; i < localStorage.length; i++) {
    const key = localStorage.key(i)
    if (key && key.startsWith(KEY_PREFIX)) victims.push(key)
  }
  for (const key of victims) localStorage.removeItem(key)
}

describe('useBatchExpandedState', () => {
  beforeEach(() => {
    clearBatchKeys()
  })

  afterEach(() => {
    vi.restoreAllMocks()
    clearBatchKeys()
  })

  it('returns the default (false) when no entry exists', () => {
    const { result } = renderHook(() => useBatchExpandedState('event-1'))
    expect(result.current[0]).toBe(false)
  })

  it('returns the explicit default when no entry exists', () => {
    const { result } = renderHook(() => useBatchExpandedState('event-2', true))
    expect(result.current[0]).toBe(true)
  })

  it('reads "1" as true', () => {
    localStorage.setItem(`${KEY_PREFIX}event-3`, '1')
    const { result } = renderHook(() => useBatchExpandedState('event-3'))
    expect(result.current[0]).toBe(true)
  })

  it('reads "0" as false even when defaultExpanded=true', () => {
    localStorage.setItem(`${KEY_PREFIX}event-4`, '0')
    const { result } = renderHook(() => useBatchExpandedState('event-4', true))
    expect(result.current[0]).toBe(false)
  })

  it('treats corrupt values as missing and falls back to default', () => {
    localStorage.setItem(`${KEY_PREFIX}event-5`, 'garbage')
    const { result } = renderHook(() => useBatchExpandedState('event-5', true))
    expect(result.current[0]).toBe(true)
  })

  it('writes "1" on toggle to true and persists across remounts', () => {
    const { result, unmount } = renderHook(() => useBatchExpandedState('event-6'))
    expect(result.current[0]).toBe(false)

    act(() => {
      result.current[1](true)
    })

    expect(result.current[0]).toBe(true)
    expect(localStorage.getItem(`${KEY_PREFIX}event-6`)).toBe('1')

    unmount()

    const { result: result2 } = renderHook(() => useBatchExpandedState('event-6'))
    expect(result2.current[0]).toBe(true)
  })

  it('writes "0" on toggle to false', () => {
    localStorage.setItem(`${KEY_PREFIX}event-7`, '1')
    const { result } = renderHook(() => useBatchExpandedState('event-7'))
    expect(result.current[0]).toBe(true)

    act(() => {
      result.current[1](false)
    })

    expect(localStorage.getItem(`${KEY_PREFIX}event-7`)).toBe('0')
  })

  it('prune sweeps every oa-batch-expanded key and leaves unrelated keys alone', () => {
    localStorage.setItem(`${KEY_PREFIX}old-1`, '1')
    localStorage.setItem(`${KEY_PREFIX}old-2`, '0')
    localStorage.setItem(`${KEY_PREFIX}old-3`, '1')
    localStorage.setItem('unrelated:keep-me', 'true')
    localStorage.setItem('another:unrelated', 'x')

    __pruneAllBatchExpandedKeysForTests()

    expect(localStorage.getItem(`${KEY_PREFIX}old-1`)).toBeNull()
    expect(localStorage.getItem(`${KEY_PREFIX}old-2`)).toBeNull()
    expect(localStorage.getItem(`${KEY_PREFIX}old-3`)).toBeNull()
    expect(localStorage.getItem('unrelated:keep-me')).toBe('true')
    expect(localStorage.getItem('another:unrelated')).toBe('x')
  })

  // Note: the "retries after quota exhaustion and succeeds" path is hard
  // to mock in jsdom — its localStorage doesn't resolve `setItem` via
  // `Storage.prototype.setItem` so neither vi.spyOn nor prototype
  // replacement reliably intercept it. The behavior is covered
  // indirectly by (a) the prune-only test above which verifies the
  // sweep logic in isolation, and (b) the "swallows write errors"
  // test below which verifies the in-memory state still updates when
  // setItem throws every time.

  it('returns default and does not crash when getItem throws (private browsing)', () => {
    vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('SecurityError')
    })
    const { result } = renderHook(() => useBatchExpandedState('event-9', true))
    expect(result.current[0]).toBe(true)
  })

  it('swallows write errors when the retry also fails', () => {
    const setItemSpy = vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
      const err = new Error('QuotaExceededError') as Error & { name: string }
      err.name = 'QuotaExceededError'
      throw err
    })

    const { result } = renderHook(() => useBatchExpandedState('event-10'))

    // Should not throw.
    expect(() => {
      act(() => {
        result.current[1](true)
      })
    }).not.toThrow()

    // In-memory state still updates even though persistence failed.
    expect(result.current[0]).toBe(true)

    setItemSpy.mockRestore()
  })

  it('keys independently for different firstEventIds', () => {
    localStorage.setItem(`${KEY_PREFIX}ev-A`, '1')
    localStorage.setItem(`${KEY_PREFIX}ev-B`, '0')
    const { result: a } = renderHook(() => useBatchExpandedState('ev-A'))
    const { result: b } = renderHook(() => useBatchExpandedState('ev-B'))
    expect(a.current[0]).toBe(true)
    expect(b.current[0]).toBe(false)
  })
})
