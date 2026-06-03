import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { usePendingEscalations, useEscalationCount } from './useEscalations'

vi.mock('../lib/rpc', () => ({
  rpcCall: vi.fn(),
}))

import { rpcCall } from '../lib/rpc'
const mockRpcCall = vi.mocked(rpcCall)

function createWrapper() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  return ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
}

beforeEach(() => {
  vi.clearAllMocks()
})

describe('usePendingEscalations', () => {
  it('fetches escalations via escalation.listPending', async () => {
    const mockEscalations = [
      { id: 'esc-1', title: 'Approve budget', urgency: 'high', status: 'pending', createdAt: '2026-01-01T00:00:00Z' },
    ]
    mockRpcCall.mockResolvedValueOnce({ escalations: mockEscalations })

    const { result } = renderHook(() => usePendingEscalations('proj-1'), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.data).toBeDefined())

    expect(mockRpcCall).toHaveBeenCalledWith('escalation.listPending', { projectId: 'proj-1' })
    expect(result.current.data).toHaveLength(1)
    expect(result.current.data![0].title).toBe('Approve budget')
  })

  it('returns empty array when no projectId', () => {
    const { result } = renderHook(() => usePendingEscalations(null), { wrapper: createWrapper() })
    expect(result.current.data).toBeUndefined()
    expect(result.current.fetchStatus).toBe('idle')
  })
})

describe('useEscalationCount', () => {
  it('fetches count via escalation.countPending', async () => {
    mockRpcCall.mockResolvedValueOnce({ count: 5 })

    const { result } = renderHook(() => useEscalationCount('proj-1'), { wrapper: createWrapper() })

    await waitFor(() => expect(result.current.data).toBeDefined())

    expect(mockRpcCall).toHaveBeenCalledWith('escalation.countPending', { projectId: 'proj-1' })
    expect(result.current.data).toBe(5)
  })
})
