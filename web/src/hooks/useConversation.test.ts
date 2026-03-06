import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor, act } from '@testing-library/react'
import { createWrapper } from '../test/test-utils'
import type { Item, AgentMessageContent } from '../lib/types'

// Shared notification registry
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

const { useConversation } = await import('./useConversation')

function makeItem(overrides: Partial<Item> = {}): Item {
  return {
    id: 'item-1',
    turnId: 'turn-1',
    type: 'user_message',
    status: 'completed',
    content: { text: 'Hello' },
    ...overrides,
  }
}

describe('useConversation', () => {
  beforeEach(() => {
    mockRpcCall.mockReset()
    notificationHandlers.clear()
  })

  it('does not fetch when threadId is null', () => {
    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useConversation(null), { wrapper: Wrapper })
    expect(result.current.data).toBeUndefined()
    expect(mockRpcCall).not.toHaveBeenCalled()
  })

  it('fetches items via item.list RPC', async () => {
    const items = [
      makeItem({ id: 'item-1', turnId: 'turn-1', type: 'user_message' }),
      makeItem({ id: 'item-2', turnId: 'turn-1', type: 'agent_message', content: { text: 'Hi there' } }),
    ]
    mockRpcCall.mockResolvedValueOnce({ items })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))

    expect(mockRpcCall).toHaveBeenCalledWith('item.list', {
      threadId: 'thread-1',
      limit: 200,
    })
    expect(result.current.data).toHaveLength(2)
  })

  it('handles empty item list from API', async () => {
    mockRpcCall.mockResolvedValueOnce({ items: [] })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    expect(result.current.data).toEqual([])
  })

  it('adds new items from item.started notifications', async () => {
    const initialItems = [makeItem({ id: 'item-1', turnId: 'turn-1' })]
    mockRpcCall.mockResolvedValueOnce({ items: initialItems })

    const { Wrapper, queryClient } = createWrapper()
    const { result } = renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(result.current.isSuccess).toBe(true))
    // Wait for the useEffect to register notification handlers
    await waitFor(() => expect(notificationHandlers.get('item.started')?.length).toBeGreaterThan(0))

    const newItem = makeItem({
      id: 'item-2',
      turnId: 'turn-1',
      type: 'agent_message',
      status: 'started',
      content: { text: '' },
    })

    act(() => {
      dispatch('item.started', { item: newItem })
    })

    // Verify cache was updated
    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    expect(cached).toHaveLength(2)
    expect(cached![1].id).toBe('item-2')
  })

  it('replaces optimistic items on item.started', async () => {
    mockRpcCall.mockResolvedValueOnce({ items: [] })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('item.started')?.length).toBeGreaterThan(0))

    // Manually inject an optimistic item
    act(() => {
      queryClient.setQueryData<Item[]>(['item.list', 'thread-1'], [
        makeItem({
          id: 'optimistic-123',
          turnId: 'turn-5',
          type: 'user_message',
          content: { text: 'Hello' },
        }),
      ])
    })

    // Real item arrives for the same turn and type
    const realItem = makeItem({
      id: 'real-item-1',
      turnId: 'turn-5',
      type: 'user_message',
      content: { text: 'Hello' },
    })
    act(() => {
      dispatch('item.started', { item: realItem })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    expect(cached).toHaveLength(1)
    expect(cached![0].id).toBe('real-item-1')
  })

  it('updates items from item.completed notifications', async () => {
    const initialItems = [
      makeItem({ id: 'item-1', turnId: 'turn-1', type: 'agent_message', status: 'started', content: { text: '' } }),
    ]
    mockRpcCall.mockResolvedValueOnce({ items: initialItems })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('item.completed')?.length).toBeGreaterThan(0))

    // Register the turn ID first via item.started (the item.completed handler
    // guards against unknown turn IDs, and the initial cache seed may run
    // before the query resolves)
    act(() => {
      dispatch('item.started', {
        item: makeItem({ id: 'item-1', turnId: 'turn-1', type: 'agent_message', status: 'started', content: { text: '' } }),
      })
    })

    const completedItem = makeItem({
      id: 'item-1',
      turnId: 'turn-1',
      type: 'agent_message',
      status: 'completed',
      content: { text: 'Full response' },
    })
    act(() => {
      dispatch('item.completed', { item: completedItem })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    expect(cached![0].status).toBe('completed')
    expect((cached![0].content as AgentMessageContent).text).toBe('Full response')
  })

  it('accumulates text deltas from item.delta notifications', async () => {
    const initialItems = [
      makeItem({
        id: 'item-1',
        turnId: 'turn-1',
        type: 'agent_message',
        status: 'started',
        content: { text: 'Hello' },
      }),
    ]
    mockRpcCall.mockResolvedValueOnce({ items: initialItems })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('item.delta')?.length).toBeGreaterThan(0))

    act(() => {
      dispatch('item.delta', {
        itemId: 'item-1',
        delta: { textDelta: ' world' },
      })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    const content = cached![0].content as AgentMessageContent
    expect(content.text).toBe('Hello world')
    expect(content.isPartial).toBe(true)
  })

  it('accumulates reasoning deltas', async () => {
    const initialItems = [
      makeItem({
        id: 'item-1',
        turnId: 'turn-1',
        type: 'reasoning',
        status: 'started',
        content: { summary: 'Think' },
      }),
    ]
    mockRpcCall.mockResolvedValueOnce({ items: initialItems })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('item.delta')?.length).toBeGreaterThan(0))

    act(() => {
      dispatch('item.delta', {
        itemId: 'item-1',
        delta: { reasoningDelta: 'ing...' },
      })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    const content = cached![0].content as { summary: string }
    expect(content.summary).toBe('Thinking...')
  })

  it('transitions item status from started to streaming on delta', async () => {
    const initialItems = [
      makeItem({
        id: 'item-1',
        turnId: 'turn-1',
        type: 'agent_message',
        status: 'started',
        content: { text: '' },
      }),
    ]
    mockRpcCall.mockResolvedValueOnce({ items: initialItems })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('item.delta')?.length).toBeGreaterThan(0))

    act(() => {
      dispatch('item.delta', {
        itemId: 'item-1',
        delta: { textDelta: 'Hello' },
      })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    expect(cached![0].status).toBe('streaming')
  })

  it('registers turn IDs from turn.started notifications', async () => {
    mockRpcCall.mockResolvedValueOnce({ items: [] })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('turn.started')?.length).toBeGreaterThan(0))

    // Register a turn
    act(() => {
      dispatch('turn.started', { turn: { id: 'turn-new', threadId: 'thread-1' } })
    })

    // Add an item for that turn
    const newItem = makeItem({
      id: 'item-new',
      turnId: 'turn-new',
      type: 'agent_message',
      status: 'started',
      content: { text: '' },
    })
    act(() => {
      dispatch('item.started', { item: newItem })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    expect(cached).toHaveLength(1)
    expect(cached![0].id).toBe('item-new')
  })

  it('does not duplicate items on repeated item.started notifications', async () => {
    mockRpcCall.mockResolvedValueOnce({ items: [] })

    const { Wrapper, queryClient } = createWrapper()
    renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => expect(notificationHandlers.get('item.started')?.length).toBeGreaterThan(0))

    const item = makeItem({ id: 'item-1', turnId: 'turn-1' })

    act(() => {
      dispatch('item.started', { item })
    })
    act(() => {
      dispatch('item.started', { item })
    })

    const cached = queryClient.getQueryData<Item[]>(['item.list', 'thread-1'])
    expect(cached).toHaveLength(1)
  })

  it('cleans up all notification listeners on unmount', async () => {
    mockRpcCall.mockResolvedValueOnce({ items: [] })

    const { Wrapper } = createWrapper()
    const { unmount } = renderHook(() => useConversation('thread-1'), { wrapper: Wrapper })

    await waitFor(() => {
      expect(notificationHandlers.get('turn.started')?.length).toBeGreaterThan(0)
    })

    unmount()

    expect(notificationHandlers.get('turn.started')?.length).toBe(0)
    expect(notificationHandlers.get('item.started')?.length).toBe(0)
    expect(notificationHandlers.get('item.completed')?.length).toBe(0)
    expect(notificationHandlers.get('item.delta')?.length).toBe(0)
  })
})
