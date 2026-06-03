import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useCreateKeyResult, useUpdateKeyResult } from './useMissions'

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

describe('mission KR mutations', () => {
  it('normalizes legacy UI measurement names before creating a key result', async () => {
    mockRpcCall.mockResolvedValueOnce({ keyResult: { id: 'kr-1', missionId: 'mission-1' } })

    const { result } = renderHook(() => useCreateKeyResult(), { wrapper: createWrapper() })

    result.current.mutate({
      missionId: 'mission-1',
      title: 'Revenue',
      measurementType: 'currency',
      direction: 'increase',
      targetValue: 100,
      unit: 'USD',
    })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())

    expect(mockRpcCall).toHaveBeenCalledWith('mission.kr.create', {
      missionId: 'mission-1',
      title: 'Revenue',
      measurementType: 'number',
      direction: 'increase',
      targetValue: 100,
      unit: 'USD',
    })
  })

  it('drops direction when creating a boolean key result', async () => {
    mockRpcCall.mockResolvedValueOnce({ keyResult: { id: 'kr-1', missionId: 'mission-1' } })

    const { result } = renderHook(() => useCreateKeyResult(), { wrapper: createWrapper() })

    result.current.mutate({
      missionId: 'mission-1',
      title: 'Launch complete',
      measurementType: 'binary',
      direction: 'increase',
      targetValue: 1,
    })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())

    expect(mockRpcCall).toHaveBeenCalledWith('mission.kr.create', {
      missionId: 'mission-1',
      title: 'Launch complete',
      measurementType: 'boolean',
      targetValue: 1,
    })
  })

  it('normalizes legacy UI measurement names before updating a key result', async () => {
    mockRpcCall.mockResolvedValueOnce({ keyResult: { id: 'kr-1', missionId: 'mission-1' } })

    const { result } = renderHook(() => useUpdateKeyResult(), { wrapper: createWrapper() })

    result.current.mutate({
      keyResultId: 'kr-1',
      missionId: 'mission-1',
      measurementType: 'numeric',
      direction: 'decrease',
      targetValue: 10,
    })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())

    expect(mockRpcCall).toHaveBeenCalledWith('mission.kr.update', {
      keyResultId: 'kr-1',
      measurementType: 'number',
      direction: 'decrease',
      targetValue: 10,
    })
  })
})
