import { act, renderHook, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import { createWrapper } from '../test/test-utils'

type Notification = { jsonrpc: '2.0'; method: string; params?: unknown }
type NotificationHandler = (notification: Notification) => void

const notificationHandlers = new Map<string, NotificationHandler[]>()
const mockRpcCall = vi.fn()

function dispatch(method: string, params: unknown = {}) {
  const handlers = notificationHandlers.get(method) ?? []
  handlers.forEach((handler) => handler({ jsonrpc: '2.0', method, params }))
}

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  onNotification: (method: string, handler: NotificationHandler) => {
    const handlers = notificationHandlers.get(method) ?? []
    handlers.push(handler)
    notificationHandlers.set(method, handlers)
    return () => {
      const current = notificationHandlers.get(method) ?? []
      const index = current.indexOf(handler)
      if (index !== -1) current.splice(index, 1)
    }
  },
}))

const { useSpaceDetail } = await import('./useSpaceDetail')

describe('useSpaceDetail live refresh', () => {
  beforeEach(() => {
    notificationHandlers.clear()
    mockRpcCall.mockReset()
    mockRpcCall.mockResolvedValue({ space: { id: 'space-1' }, entries: [] })
  })

  it('hydrates from space.detail and refreshes from durable event notifications', async () => {
    const { Wrapper } = createWrapper()
    renderHook(() => useSpaceDetail('space-1'), { wrapper: Wrapper })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(1))

    await act(async () => {
      dispatch('event.append', { eventId: 'evt-1' })
    })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(2))
    expect(mockRpcCall).toHaveBeenLastCalledWith('space.detail', {
      spaceId: 'space-1',
      limit: 2000,
    })
  })

  it('ignores durable event notifications for other spaces', async () => {
    const { Wrapper } = createWrapper()
    renderHook(() => useSpaceDetail('space-1'), { wrapper: Wrapper })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(1))

    await act(async () => {
      dispatch('event.append', { spaceId: 'space-2', eventId: 'evt-2' })
    })

    await new Promise(resolve => setTimeout(resolve, 100))
    expect(mockRpcCall).toHaveBeenCalledTimes(1)
  })

  it('refreshes selected channel only for matching channel notifications', async () => {
    mockRpcCall.mockResolvedValue({ messages: [], activities: [] })
    const { Wrapper } = createWrapper()
    renderHook(() => useSpaceDetail('space-1', 'channel:space-1:member:member-1'), { wrapper: Wrapper })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(1))

    await act(async () => {
      dispatch('event.append', { spaceId: 'space-1', channelId: 'channel:space-1:member:member-2' })
    })
    await new Promise(resolve => setTimeout(resolve, 300))
    expect(mockRpcCall).toHaveBeenCalledTimes(1)

    await act(async () => {
      dispatch('event.append', { spaceId: 'space-1', type: 'agent.turn.started' })
    })
    await new Promise(resolve => setTimeout(resolve, 300))
    expect(mockRpcCall).toHaveBeenCalledTimes(1)

    await act(async () => {
      dispatch('event.append', { spaceId: 'space-1', channelId: 'channel:space-1:member:member-1' })
    })
    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(2))
    expect(mockRpcCall).toHaveBeenLastCalledWith('message.conversation.list', {
      channelId: 'channel:space-1:member:member-1',
      limit: 2000,
    })
  })

  it('does not subscribe to legacy turn/item notifications', async () => {
    const { Wrapper } = createWrapper()
    renderHook(() => useSpaceDetail('space-1'), { wrapper: Wrapper })

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(1))

    await act(async () => {
      dispatch('turn.started', { turn: { id: 'turn-1' } })
      dispatch('item.delta', { itemId: 'item-1', delta: { textDelta: 'hello' } })
      dispatch('item.completed', { item: { id: 'item-1', turnId: 'turn-1', type: 'agent_message', status: 'completed' } })
    })
    expect(mockRpcCall).toHaveBeenCalledTimes(1)
  })
})
