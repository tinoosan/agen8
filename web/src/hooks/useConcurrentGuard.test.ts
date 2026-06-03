import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, act } from '@testing-library/react'
import { useConcurrentGuard } from './useConcurrentGuard'

describe('useConcurrentGuard', () => {
  beforeEach(() => {
    vi.useFakeTimers()
  })

  afterEach(() => {
    vi.useRealTimers()
  })

  it('allows first submit and sets submitting to true', async () => {
    const { result } = renderHook(() => useConcurrentGuard())

    expect(result.current.isSubmitting).toBe(false)

    let resolved = false
    await act(async () => {
      await result.current.guard(async () => { resolved = true })
    })

    // After the synchronous action completes, submitting resets to false
    expect(result.current.isSubmitting).toBe(false)
    // The async action was called
    expect(resolved).toBe(true)
  })

  it('prevents double-submit while action is pending', async () => {
    const { result } = renderHook(() => useConcurrentGuard())

    let callCount = 0
    const slowAction = () => new Promise<void>(resolve => {
      callCount++
      setTimeout(resolve, 1000)
    })

    // First submit — don't await, so it stays in-flight
    await act(async () => {
      result.current.guard(slowAction)
    })

    expect(callCount).toBe(1)
    expect(result.current.isSubmitting).toBe(true)

    // Second submit while first is pending — should be rejected
    await act(async () => {
      result.current.guard(slowAction)
    })

    expect(callCount).toBe(1) // Still 1 — second was blocked
  })

  it('resets isSubmitting after action completes', async () => {
    const { result } = renderHook(() => useConcurrentGuard())

    let resolveFn: (() => void) | undefined
    const action = () => new Promise<void>(resolve => { resolveFn = resolve })

    await act(async () => {
      result.current.guard(action)
    })

    expect(result.current.isSubmitting).toBe(true)

    await act(async () => {
      resolveFn!()
    })

    expect(result.current.isSubmitting).toBe(false)
  })

  it('resets isSubmitting after action fails and rethrows', async () => {
    const { result } = renderHook(() => useConcurrentGuard())

    await act(async () => {
      try {
        await result.current.guard(async () => {
          throw new Error('network error')
        })
        expect.fail('guard should have rethrown')
      } catch (err) {
        expect((err as Error).message).toBe('network error')
      }
    })

    expect(result.current.isSubmitting).toBe(false)
    expect(result.current.error).toBe('network error')
  })

  it('captures "already resolved" error and rethrows', async () => {
    const { result } = renderHook(() => useConcurrentGuard())

    let caught: Error | undefined
    await act(async () => {
      try {
        await result.current.guard(async () => {
          throw new Error('already resolved')
        })
      } catch (err) {
        caught = err as Error
      }
    })

    expect(caught?.message).toBe('already resolved')
    expect(result.current.isSubmitting).toBe(false)
    expect(result.current.error).toBe('already resolved')
  })

  it('clearError resets the error state', async () => {
    const { result } = renderHook(() => useConcurrentGuard())

    await act(async () => {
      try {
        await result.current.guard(async () => {
          throw new Error('some error')
        })
      } catch {
        // Expected — guard rethrows
      }
    })

    expect(result.current.error).toBe('some error')

    act(() => {
      result.current.clearError()
    })

    expect(result.current.error).toBeNull()
  })
})
