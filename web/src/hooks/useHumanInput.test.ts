import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useHumanInput } from './useHumanInput'

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
  vi.useRealTimers()
  vi.clearAllMocks()
  notificationHandlers.clear()
})

describe('useHumanInput', () => {
  it('loads pending human input for the selected channel', async () => {
    mockRpcCall.mockResolvedValueOnce({
      pending: {
        spaceId: 'space-1',
        memberId: 'member-1',
        channelId: 'channel-1',
        toolCallId: 'call-1',
        toolName: 'decision',
        primitive: 'questions',
        payload: { questions: [{ id: 'q1', text: 'Choose', type: 'multiple_choice', options: ['A'] }] },
        projectId: 'project-1',
        createdAt: '2026-05-27T10:00:00Z',
      },
    })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useHumanInput('channel-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.pending?.toolCallId).toBe('call-1'))
    expect(mockRpcCall).toHaveBeenCalledWith('channel.human_input.pending', { channelId: 'channel-1' })
  })

  it('submits answers with the asker identity from the pending row', async () => {
    mockRpcCall.mockResolvedValueOnce({ pending: null })
    mockRpcCall.mockResolvedValueOnce({ ok: true })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useHumanInput('channel-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.query.isSuccess).toBe(true))
    await act(async () => {
      await result.current.submit.mutateAsync({
        spaceId: 'space-1',
        memberId: 'member-1',
        toolCallId: 'call-1',
        result: { answers: [{ questionId: 'q1', selectedOption: 'A' }] },
      })
    })

    expect(mockRpcCall).toHaveBeenCalledWith('channel.human_input.submit', {
      spaceId: 'space-1',
      memberId: 'member-1',
      toolCallId: 'call-1',
      result: { answers: [{ questionId: 'q1', selectedOption: 'A' }] },
    })
  })

  it('invalidates the pending query when a channel human-input notification arrives', () => {
    mockRpcCall.mockResolvedValue({ pending: null })
    const { Wrapper, queryClient } = createWrapper()
    const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

    renderHook(() => useHumanInput('channel-1'), { wrapper: Wrapper })

    act(() => {
      notificationHandlers.get('channel.human_input.changed')?.forEach((handler) =>
        handler({ params: { channelId: 'channel-1' } }),
      )
    })

    expect(invalidateSpy).toHaveBeenCalledWith({ queryKey: ['channel.human_input.pending', 'channel-1'] })
  })

  it('does not poll for pending input without backend notifications', async () => {
    mockRpcCall.mockResolvedValue({ pending: null })

    const { Wrapper } = createWrapper()
    renderHook(() => useHumanInput('channel-1'), { wrapper: Wrapper })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(1))
    vi.useFakeTimers()
    await act(async () => {
      await vi.advanceTimersByTimeAsync(5000)
    })

    expect(mockRpcCall).toHaveBeenCalledTimes(1)
  })
})
