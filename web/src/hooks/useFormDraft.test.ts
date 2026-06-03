import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useFormDraft } from './useFormDraft'

function createMockStorage(): Storage & { store: Map<string, string> } {
  const store = new Map<string, string>()
  return {
    store,
    getItem: vi.fn((key: string) => store.get(key) ?? null),
    setItem: vi.fn((key: string, value: string) => { store.set(key, value) }),
    removeItem: vi.fn((key: string) => { store.delete(key) }),
    clear: vi.fn(() => { store.clear() }),
    get length() { return store.size },
    key: vi.fn(() => null),
  }
}

describe('useFormDraft', () => {
  let mockStorage: ReturnType<typeof createMockStorage>

  beforeEach(() => {
    vi.useFakeTimers()
    mockStorage = createMockStorage()
    vi.stubGlobal('localStorage', mockStorage)
  })

  afterEach(() => {
    vi.useRealTimers()
    vi.unstubAllGlobals()
  })

  it('returns initial values when no draft exists', () => {
    const initial = { title: '', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))
    expect(result.current.values).toEqual(initial)
    expect(result.current.hasDraft).toBe(false)
  })

  it('restores saved draft from localStorage on mount', () => {
    const draft = { title: 'saved title', note: 'saved note' }
    mockStorage.setItem('oa-draft-test-key', JSON.stringify(draft))

    const initial = { title: '', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))
    expect(result.current.values).toEqual(draft)
    expect(result.current.hasDraft).toBe(true)
  })

  it('saves form state to localStorage after debounce on update', () => {
    const initial = { title: '', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))

    act(() => {
      result.current.update({ title: 'new title' })
    })

    // Should not save immediately (debounced)
    expect(mockStorage.setItem).not.toHaveBeenCalledWith(
      'oa-draft-test-key',
      expect.any(String),
    )

    // Advance past debounce interval
    act(() => { vi.advanceTimersByTime(1500) })

    expect(mockStorage.setItem).toHaveBeenCalledWith(
      'oa-draft-test-key',
      JSON.stringify({ title: 'new title', note: '' }),
    )
  })

  it('clears saved state on successful submit via clear()', () => {
    const draft = { title: 'saved', note: 'draft' }
    mockStorage.setItem('oa-draft-test-key', JSON.stringify(draft))

    const initial = { title: '', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))

    act(() => {
      result.current.clear()
    })

    expect(mockStorage.removeItem).toHaveBeenCalledWith('oa-draft-test-key')
    expect(result.current.values).toEqual(initial)
    expect(result.current.hasDraft).toBe(false)
  })

  it('handles corrupted JSON in localStorage gracefully', () => {
    mockStorage.setItem('oa-draft-test-key', 'not valid json{{{')

    const initial = { title: 'default', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))

    // Should fall back to initial values without crashing
    expect(result.current.values).toEqual(initial)
    expect(result.current.hasDraft).toBe(false)
  })

  it('handles localStorage throwing (private browsing) gracefully', () => {
    const throwStorage = {
      getItem: vi.fn(() => { throw new DOMException('Blocked') }),
      setItem: vi.fn(() => { throw new DOMException('Blocked') }),
      removeItem: vi.fn(() => { throw new DOMException('Blocked') }),
      clear: vi.fn(),
      length: 0,
      key: vi.fn(() => null),
    }
    vi.stubGlobal('localStorage', throwStorage)

    const initial = { title: '', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))

    // Should not crash
    expect(result.current.values).toEqual(initial)
    expect(result.current.hasDraft).toBe(false)

    // update should not throw
    act(() => {
      result.current.update({ title: 'test' })
    })
    // hasDraft is optimistically true before debounce fires
    expect(result.current.hasDraft).toBe(true)

    // After debounce fires and setItem throws, hasDraft reverts to false
    act(() => { vi.advanceTimersByTime(1500) })
    expect(result.current.hasDraft).toBe(false)
  })

  it('merges partial updates into existing values', () => {
    const initial = { title: '', note: '', extra: 'keep' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))

    act(() => {
      result.current.update({ title: 'updated' })
    })

    expect(result.current.values).toEqual({ title: 'updated', note: '', extra: 'keep' })
  })

  it('re-initializes when key changes (different task)', () => {
    const draftA = { title: 'draft A', note: '' }
    const draftB = { title: 'draft B', note: 'note B' }
    mockStorage.setItem('oa-draft-key-a', JSON.stringify(draftA))
    mockStorage.setItem('oa-draft-key-b', JSON.stringify(draftB))

    const initial = { title: '', note: '' }
    const { result, rerender } = renderHook(
      ({ formKey }: { formKey: string }) => useFormDraft(formKey, initial),
      { initialProps: { formKey: 'key-a' } },
    )

    expect(result.current.values).toEqual(draftA)
    expect(result.current.hasDraft).toBe(true)

    // Switch to key-b — should load draft B
    rerender({ formKey: 'key-b' })
    expect(result.current.values).toEqual(draftB)
    expect(result.current.hasDraft).toBe(true)

    // Switch to key-c (no draft) — should reset to initial
    rerender({ formKey: 'key-c' })
    expect(result.current.values).toEqual(initial)
    expect(result.current.hasDraft).toBe(false)
  })

  it('debounces rapid updates — only last state is saved', () => {
    const initial = { title: '', note: '' }
    const { result } = renderHook(() => useFormDraft('test-key', initial))

    act(() => {
      result.current.update({ title: 'a' })
    })
    act(() => {
      result.current.update({ title: 'ab' })
    })
    act(() => {
      result.current.update({ title: 'abc' })
    })

    act(() => { vi.advanceTimersByTime(1500) })

    // Only the final state should be saved
    const calls = (mockStorage.setItem as ReturnType<typeof vi.fn>).mock.calls
    const draftCalls = calls.filter(([key]: [string]) => key === 'oa-draft-test-key')
    expect(draftCalls).toHaveLength(1)
    expect(JSON.parse(draftCalls[0][1])).toEqual({ title: 'abc', note: '' })
  })
})
