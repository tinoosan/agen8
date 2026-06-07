import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useListChangeSignals } from './useListChangeSignals'

type Item = { id: string; status: string }

// jsdom has no matchMedia; default it to "motion allowed". Individual tests
// override matches to exercise the reduced-motion branch.
function stubMatchMedia(reduced: boolean) {
  window.matchMedia = vi.fn().mockImplementation((query: string) => ({
    matches: reduced,
    media: query,
    onchange: null,
    addEventListener: vi.fn(),
    removeEventListener: vi.fn(),
    addListener: vi.fn(),
    removeListener: vi.fn(),
    dispatchEvent: vi.fn(),
  }))
}

const key = (i: Item) => i.id
const sig = (i: Item) => i.status

// setState is deferred into requestAnimationFrame; under fake timers rAF is a
// queued timer, so flush it (plus microtasks) to observe the resulting state.
function flushRaf() {
  act(() => {
    vi.advanceTimersByTime(20)
  })
}

describe('useListChangeSignals', () => {
  beforeEach(() => {
    vi.useFakeTimers()
    stubMatchMedia(false)
  })
  afterEach(() => {
    vi.runOnlyPendingTimers()
    vi.useRealTimers()
  })

  it('announces nothing on the first load (baseline only)', () => {
    const { result } = renderHook(
      ({ items }) => useListChangeSignals(items, key, sig),
      { initialProps: { items: [{ id: 'a', status: 'active' }] as Item[] } },
    )
    flushRaf()
    expect([...result.current.changedIds]).toEqual([])
  })

  it('does not flag a newly-arrived id (entrance is CSS, not a change)', () => {
    const { result, rerender } = renderHook(
      ({ items }) => useListChangeSignals(items, key, sig),
      { initialProps: { items: [{ id: 'a', status: 'active' }] as Item[] } },
    )
    act(() => {
      rerender({ items: [{ id: 'b', status: 'active' }, { id: 'a', status: 'active' }] })
    })
    flushRaf()
    expect([...result.current.changedIds]).toEqual([])
  })

  it('flags an id whose signal changed', () => {
    const { result, rerender } = renderHook(
      ({ items }) => useListChangeSignals(items, key, sig),
      { initialProps: { items: [{ id: 'a', status: 'active' }] as Item[] } },
    )
    act(() => {
      rerender({ items: [{ id: 'a', status: 'in_review' }] })
    })
    flushRaf()
    expect([...result.current.changedIds]).toEqual(['a'])
  })

  it('clears the changed flag after the flash window elapses', () => {
    const { result, rerender } = renderHook(
      ({ items }) => useListChangeSignals(items, key, sig, { flashMs: 1000 }),
      { initialProps: { items: [{ id: 'a', status: 'active' }] as Item[] } },
    )
    act(() => {
      rerender({ items: [{ id: 'a', status: 'blocked' }] })
    })
    flushRaf()
    expect([...result.current.changedIds]).toEqual(['a'])
    act(() => {
      vi.advanceTimersByTime(1001)
    })
    expect([...result.current.changedIds]).toEqual([])
  })

  it('emits no signals when reduced motion is preferred', () => {
    stubMatchMedia(true)
    const { result, rerender } = renderHook(
      ({ items }) => useListChangeSignals(items, key, sig),
      { initialProps: { items: [{ id: 'a', status: 'active' }] as Item[] } },
    )
    act(() => {
      rerender({ items: [{ id: 'a', status: 'in_review' }] })
    })
    flushRaf()
    expect([...result.current.changedIds]).toEqual([])
  })
})
