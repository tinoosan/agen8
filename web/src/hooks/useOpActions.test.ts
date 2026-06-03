import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import {
  usePendingOpActions,
  useStrategyMapOpActions,
  useOpAction,
  useOpActionCounts,
  useOpActionSSE,
} from './useOpActions'

const notificationHandlers = new Map<string, Array<(notif: Record<string, unknown>) => void>>()

vi.mock('../lib/rpc', () => ({
  rpcCall: vi.fn(),
  onNotification: (method: string, handler: (notif: Record<string, unknown>) => void) => {
    if (!notificationHandlers.has(method)) notificationHandlers.set(method, [])
    notificationHandlers.get(method)!.push(handler)
    return () => {
      const list = notificationHandlers.get(method)
      if (!list) return
      const idx = list.indexOf(handler)
      if (idx !== -1) list.splice(idx, 1)
    }
  },
}))

import { rpcCall } from '../lib/rpc'
const mockRpcCall = vi.mocked(rpcCall)

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const Wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
  return { Wrapper, queryClient }
}

beforeEach(() => {
  vi.clearAllMocks()
  notificationHandlers.clear()
})

describe('usePendingOpActions', () => {
  it('fetches actions via opAction.listPending', async () => {
    const mockActions = [
      { id: 'oa-1', title: 'Sign contract', status: 'pending', blocking: true, createdAt: '2026-01-01T00:00:00Z' },
    ]
    mockRpcCall.mockResolvedValueOnce({ opActions: mockActions })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => usePendingOpActions('proj-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.data).toBeDefined())

    expect(mockRpcCall).toHaveBeenCalledWith('opAction.listPending', { projectId: 'proj-1' })
    expect(result.current.data).toHaveLength(1)
    expect(result.current.data![0].title).toBe('Sign contract')
  })

  it('is disabled when projectId is null', () => {
    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => usePendingOpActions(null), { wrapper: Wrapper })
    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useStrategyMapOpActions', () => {
  it('fetches active+completed OAs via opAction.list and excludes canceled', async () => {
    const mockActions = [
      { id: 'oa-1', title: 'A', status: 'completed', blocking: false, createdAt: '2026-01-01T00:00:00Z' },
      { id: 'oa-2', title: 'B', status: 'in_progress', blocking: false, createdAt: '2026-01-01T00:00:00Z' },
      { id: 'oa-3', title: 'C', status: 'canceled', blocking: false, createdAt: '2026-01-01T00:00:00Z' },
    ]
    mockRpcCall.mockResolvedValueOnce({ opActions: mockActions })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useStrategyMapOpActions('proj-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.data).toBeDefined())

    expect(mockRpcCall).toHaveBeenCalledWith('opAction.list', {
      projectId: 'proj-1',
      status: ['pending', 'acknowledged', 'in_progress', 'pending_verification', 'blocked', 'completed'],
      limit: 200,
    })
    expect(result.current.data?.map((oa) => oa.id)).toEqual(['oa-1', 'oa-2'])
  })

  it('is disabled when projectId is null', () => {
    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useStrategyMapOpActions(null), { wrapper: Wrapper })
    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useOpAction', () => {
  it('fetches single action via opAction.get', async () => {
    const mockAction = { id: 'oa-1', title: 'Sign contract', status: 'in_progress' }
    mockRpcCall.mockResolvedValueOnce({ opAction: mockAction })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useOpAction('oa-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.data).toBeDefined())

    expect(mockRpcCall).toHaveBeenCalledWith('opAction.get', { actionId: 'oa-1' })
    expect(result.current.data!.status).toBe('in_progress')
  })

  it('is disabled when actionId is null', () => {
    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useOpAction(null), { wrapper: Wrapper })
    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useOpActionCounts', () => {
  it('fetches status counts via opAction.countStatus', async () => {
    mockRpcCall.mockResolvedValueOnce({ counts: { pending: 3, in_progress: 2, completed: 10 } })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useOpActionCounts('proj-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.data).toBeDefined())

    expect(mockRpcCall).toHaveBeenCalledWith('opAction.countStatus', { projectId: 'proj-1' })
    expect(result.current.data!.pending).toBe(3)
    expect(result.current.data!.in_progress).toBe(2)
  })
})

describe('useOpActionSSE', () => {
  it('invalidates strategy map OA query on oa.* notifications', () => {
    const { Wrapper, queryClient } = createWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    renderHook(() => useOpActionSSE(), { wrapper: Wrapper })

    act(() => {
      notificationHandlers.get('event.append')?.forEach((h) =>
        h({ event: { type: 'oa.completed' } }),
      )
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['opAction.listStrategyMap'] })
  })
})
