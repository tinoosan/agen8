import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useActivity } from './useActivity'
import { createWrapper } from '../test/test-utils'
import type { ActivityEvent } from '../lib/types'

// Shared notification registry (plain functions, not vi.fn, to survive vi.clearAllMocks)
const notificationHandlers = new Map<string, Array<(n: { jsonrpc: '2.0'; method: string; params?: unknown }) => void>>()

function dispatch(method: string, params: unknown) {
  const list = notificationHandlers.get(method)
  if (list) list.forEach(h => h({ jsonrpc: '2.0', method, params }))
}

const mockRpcCall = vi.fn()

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  onNotification: (method: string, handler: (n: { jsonrpc: '2.0'; method: string; params?: unknown }) => void) => {
    if (!notificationHandlers.has(method)) notificationHandlers.set(method, [])
    notificationHandlers.get(method)!.push(handler)
    return () => {
      const list = notificationHandlers.get(method)
      if (list) {
        const idx = list.indexOf(handler)
        if (idx !== -1) list.splice(idx, 1)
      }
    }
  },
}))

function makeActivity(overrides: Partial<ActivityEvent> = {}): ActivityEvent {
  return {
    id: `act-${Math.random().toString(36).slice(2, 8)}`,
    kind: 'agent_message',
    title: 'Test activity',
    status: 'ok',
    startedAt: new Date().toISOString(),
    ...overrides,
  }
}

describe('useActivity', () => {
  beforeEach(() => {
    mockRpcCall.mockReset()
    notificationHandlers.clear()
  })

  it('does not fetch when threadId is null', () => {
    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useActivity({ threadId: null, teamId: 'team-1' }),
      { wrapper: Wrapper },
    )
    expect(result.current.data).toBeUndefined()
    expect(result.current.isLoading).toBe(false)
    expect(mockRpcCall).not.toHaveBeenCalled()
  })

  it('fetches activities via rpcCall when threadId is provided', async () => {
    const activities = [
      makeActivity({ id: 'a1', title: 'First' }),
      makeActivity({ id: 'a2', title: 'Second' }),
    ]
    mockRpcCall.mockResolvedValueOnce({ activities })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1', includeChildRuns: true, limit: 200 }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockRpcCall).toHaveBeenCalledWith('activity.list', expect.objectContaining({
      threadId: 'thread-1',
      teamId: 'team-1',
      includeChildRuns: true,
      limit: 200,
      offset: 0,
      sortDesc: false,
    }))
    expect(result.current.data).toHaveLength(2)
  })

  it('handles API returning empty activities', async () => {
    mockRpcCall.mockResolvedValueOnce({ activities: [] })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1' }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual([])
  })

  it('handles API returning undefined activities', async () => {
    mockRpcCall.mockResolvedValueOnce({})

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1' }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual([])
  })

  it('paginates through results using nextOffset', async () => {
    const page1 = [makeActivity({ id: 'a1' }), makeActivity({ id: 'a2' })]
    const page2 = [makeActivity({ id: 'a3' })]
    mockRpcCall
      .mockResolvedValueOnce({ activities: page1, nextOffset: 2 })
      .mockResolvedValueOnce({ activities: page2 })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1', limit: 200 }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(3)
    expect(mockRpcCall).toHaveBeenCalledTimes(2)
    // Second call should use offset from first page
    expect(mockRpcCall).toHaveBeenNthCalledWith(2, 'activity.list', expect.objectContaining({
      offset: 2,
    }))
  })

  it('deduplicates activities by id', async () => {
    const activities = [
      makeActivity({ id: 'a1', title: 'First' }),
      makeActivity({ id: 'a1', title: 'Duplicate' }),
      makeActivity({ id: 'a2', title: 'Second' }),
    ]
    mockRpcCall.mockResolvedValueOnce({ activities })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1' }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(2)
    expect(result.current.data![0].title).toBe('First')
  })

  it('invalidates queries on event.append notifications', async () => {
    mockRpcCall.mockResolvedValueOnce({ activities: [makeActivity({ id: 'a1' })] })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1' }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(notificationHandlers.get('event.append')?.length).toBeGreaterThan(0))

    const spy = vi.spyOn(queryClient, 'invalidateQueries')

    act(() => {
      dispatch('event.append', { event: { runId: 'run-1' } })
    })

    expect(spy).toHaveBeenCalled()
    spy.mockRestore()
  })

  it('cleans up notification listener on unmount', async () => {
    mockRpcCall.mockResolvedValueOnce({ activities: [] })

    const { Wrapper } = createWrapper()
    const { unmount } = renderHook(
      () => useActivity({ threadId: 'thread-1', teamId: 'team-1' }),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(notificationHandlers.get('event.append')?.length).toBeGreaterThan(0))

    unmount()

    expect(notificationHandlers.get('event.append')?.length).toBe(0)
  })
})
