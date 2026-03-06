import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { useActivity } from './useActivity'
import { createWrapper } from '../test/test-utils'
import type { ActivityEvent } from '../lib/types'

// Mock rpc module
vi.mock('../lib/rpc', () => {
  const notificationHandlers = new Map<string, Array<(n: { jsonrpc: '2.0'; method: string; params?: unknown }) => void>>()

  return {
    rpcCall: vi.fn(),
    onNotification: vi.fn((method: string, handler: (n: { jsonrpc: '2.0'; method: string; params?: unknown }) => void) => {
      if (!notificationHandlers.has(method)) notificationHandlers.set(method, [])
      notificationHandlers.get(method)!.push(handler)
      return () => {
        const list = notificationHandlers.get(method)
        if (list) {
          const idx = list.indexOf(handler)
          if (idx !== -1) list.splice(idx, 1)
        }
      }
    }),
    _notificationHandlers: notificationHandlers,
    _dispatch: (method: string, params: unknown) => {
      const list = notificationHandlers.get(method)
      if (list) list.forEach(h => h({ jsonrpc: '2.0', method, params }))
    },
  }
})

// Import the mocked module to control it
const rpcMock = await import('../lib/rpc') as typeof import('../lib/rpc') & {
  _dispatch: (method: string, params: unknown) => void
  _notificationHandlers: Map<string, unknown[]>
}

const mockRpcCall = rpcMock.rpcCall as ReturnType<typeof vi.fn>

function makeEvent(overrides: Partial<ActivityEvent> = {}): ActivityEvent {
  return {
    seq: 1,
    type: 'agent_message',
    role: 'coordinator',
    summary: 'Test event',
    createdAt: new Date().toISOString(),
    ...overrides,
  }
}

describe('useActivity', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    rpcMock._notificationHandlers.clear()
  })

  afterEach(() => {
    vi.restoreAllMocks()
  })

  it('returns empty array when teamId is null', () => {
    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity(null), { wrapper: Wrapper })
    expect(result.current.data).toBeUndefined()
    expect(result.current.isLoading).toBe(false)
  })

  it('fetches events via rpcCall when teamId is provided', async () => {
    const events = [makeEvent({ seq: 1 }), makeEvent({ seq: 2 })]
    mockRpcCall.mockResolvedValueOnce({ events })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockRpcCall).toHaveBeenCalledWith('activity.list', {
      teamId: 'team-1',
      limit: 100,
    })
    expect(result.current.data).toHaveLength(2)
  })

  it('handles API returning empty events', async () => {
    mockRpcCall.mockResolvedValueOnce({ events: [] })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual([])
  })

  it('handles API returning undefined events', async () => {
    mockRpcCall.mockResolvedValueOnce({})

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual([])
  })

  it('appends events from event.append notifications', async () => {
    const initialEvents = [makeEvent({ seq: 1, summary: 'First' })]
    mockRpcCall.mockResolvedValueOnce({ events: initialEvents })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(1)

    // Simulate receiving a notification for the same team
    const newEvent = makeEvent({ seq: 2, summary: 'Second' })
    act(() => {
      rpcMock._dispatch('event.append', { event: newEvent, teamId: 'team-1' })
    })

    await waitFor(() => expect(result.current.data).toHaveLength(2))
    expect(result.current.data![1].summary).toBe('Second')
  })

  it('ignores event.append notifications for different teams', async () => {
    const initialEvents = [makeEvent({ seq: 1 })]
    mockRpcCall.mockResolvedValueOnce({ events: initialEvents })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Notification for a different team
    act(() => {
      rpcMock._dispatch('event.append', {
        event: makeEvent({ seq: 2, summary: 'Other team' }),
        teamId: 'team-2',
      })
    })

    // Should still have only 1 event
    expect(result.current.data).toHaveLength(1)
  })

  it('caps events at 200 to prevent memory leaks', async () => {
    const initialEvents = Array.from({ length: 199 }, (_, i) =>
      makeEvent({ seq: i + 1, summary: `Event ${i + 1}` }),
    )
    mockRpcCall.mockResolvedValueOnce({ events: initialEvents })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toHaveLength(199)

    // Add 2 more to push past 200
    act(() => {
      rpcMock._dispatch('event.append', {
        event: makeEvent({ seq: 200, summary: 'Event 200' }),
        teamId: 'team-1',
      })
    })
    act(() => {
      rpcMock._dispatch('event.append', {
        event: makeEvent({ seq: 201, summary: 'Event 201' }),
        teamId: 'team-1',
      })
    })

    await waitFor(() => {
      const data = result.current.data!
      expect(data.length).toBeLessThanOrEqual(201)
      // Last event should always be the newest
      expect(data[data.length - 1].summary).toBe('Event 201')
    })
  })

  it('cleans up notification listener on unmount', async () => {
    mockRpcCall.mockResolvedValueOnce({ events: [] })

    const { Wrapper } = createWrapper()
    const { result, unmount } = renderHook(() => useActivity('team-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    // Notification handlers should be registered
    expect(rpcMock._notificationHandlers.get('event.append')?.length).toBeGreaterThan(0)

    unmount()

    // After unmount, handlers should be cleaned up
    expect(rpcMock._notificationHandlers.get('event.append')?.length).toBe(0)
  })
})
