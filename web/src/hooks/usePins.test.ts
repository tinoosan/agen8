import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { usePins, type PinView } from './usePins'

vi.mock('../lib/rpc', () => ({
  rpcCall: vi.fn(),
  rpcUnwrapList: vi.fn(),
  onNotification: vi.fn(() => () => {}),
}))

import { rpcCall, rpcUnwrapList } from '../lib/rpc'
const mockRpcCall = vi.mocked(rpcCall)
const mockRpcUnwrapList = vi.mocked(rpcUnwrapList)

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
}

function pin(nodeRef: string, nodeType = 'mission'): PinView {
  return { projectId: 'proj-1', nodeRef, nodeType, createdAt: '2026-06-07T00:00:00Z' }
}

// Wire the rpc mocks to a small in-memory store so pin.add / pin.remove mutate
// the same data pin.list reads back — i.e. the mock behaves like the real
// server. Without this, the post-mutation invalidate/refetch would clobber the
// optimistic update with stale data and tests would race.
function installStatefulRpc(initial: PinView[]) {
  let store = [...initial]
  mockRpcUnwrapList.mockImplementation(async () => [...store])
  mockRpcCall.mockImplementation(async (method: string, params: unknown) => {
    const p = params as { nodeRef: string; nodeType?: string }
    if (method === 'pin.add') {
      if (!store.some((x) => x.nodeRef === p.nodeRef)) store = [pin(p.nodeRef, p.nodeType), ...store]
    } else if (method === 'pin.remove') {
      store = store.filter((x) => x.nodeRef !== p.nodeRef)
    }
    return {}
  })
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('usePins', () => {
  it('reflects server pins via isPinned / pinnedIds', async () => {
    installStatefulRpc([pin('mission-1'), pin('dec-9', 'decision')])

    const { result } = renderHook(() => usePins('proj-1'), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.isPinned('mission-1')).toBe(true))
    expect(result.current.isPinned('dec-9')).toBe(true)
    expect(result.current.isPinned('mission-x')).toBe(false)
    expect(result.current.pinnedIds.has('mission-1')).toBe(true)
  })

  it('togglePin on an unpinned node calls pin.add with nodeRef + nodeType and pins it', async () => {
    installStatefulRpc([])

    const { result } = renderHook(() => usePins('proj-1'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.togglePin('dec-9', 'decision'))

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())
    expect(mockRpcCall).toHaveBeenCalledWith('pin.add', {
      projectId: 'proj-1',
      nodeRef: 'dec-9',
      nodeType: 'decision',
    })
    await waitFor(() => expect(result.current.isPinned('dec-9')).toBe(true))
  })

  it('togglePin on a pinned node calls pin.remove and unpins it', async () => {
    installStatefulRpc([pin('mission-1')])

    const { result } = renderHook(() => usePins('proj-1'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isPinned('mission-1')).toBe(true))

    act(() => result.current.togglePin('mission-1', 'mission'))

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())
    expect(mockRpcCall).toHaveBeenCalledWith('pin.remove', {
      projectId: 'proj-1',
      nodeRef: 'mission-1',
    })
    await waitFor(() => expect(result.current.isPinned('mission-1')).toBe(false))
  })

  it('does not query or mutate when projectId is null', async () => {
    const { result } = renderHook(() => usePins(null), { wrapper: createWrapper() })

    act(() => result.current.togglePin('mission-1', 'mission'))

    expect(mockRpcUnwrapList).not.toHaveBeenCalled()
    expect(mockRpcCall).not.toHaveBeenCalled()
  })

  it('rolls back the optimistic add when pin.add fails', async () => {
    mockRpcUnwrapList.mockResolvedValue([])
    mockRpcCall.mockRejectedValueOnce(new Error('boom'))

    const { result } = renderHook(() => usePins('proj-1'), { wrapper: createWrapper() })
    await waitFor(() => expect(result.current.isLoading).toBe(false))

    act(() => result.current.togglePin('dec-9', 'decision'))

    // Optimistic flip, then rollback after the rejection settles.
    await waitFor(() => expect(result.current.isPinned('dec-9')).toBe(false))
  })
})
