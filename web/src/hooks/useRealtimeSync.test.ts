import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { createElement } from 'react'
import { useRealtimeInvalidation } from './useRealtimeSync'
import { qk } from '../lib/queryKeys'

// Capture the handler the hook registers so the test can drive fake SSE events.
let registered: ((notif: Record<string, unknown>) => void) | null = null
const unsub = vi.fn()

vi.mock('../lib/rpc', () => ({
  onNotification: vi.fn((_method: string, handler: (n: Record<string, unknown>) => void) => {
    registered = handler
    return unsub
  }),
}))

import { onNotification } from '../lib/rpc'
const mockOnNotification = vi.mocked(onNotification)

function setup(enabled = true) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  const invalidate = vi.spyOn(queryClient, 'invalidateQueries')
  const wrapper = ({ children }: { children: React.ReactNode }) =>
    createElement(QueryClientProvider, { client: queryClient }, children)
  const view = renderHook(() => useRealtimeInvalidation(enabled), { wrapper })
  return { invalidate, view }
}

function emit(type: string) {
  registered?.({ method: 'event.append', event: { type } })
}

function invalidatedKeys(invalidate: ReturnType<typeof vi.spyOn>): string[][] {
  return invalidate.mock.calls.map((c) => (c[0] as { queryKey: unknown[] }).queryKey as string[])
}

describe('useRealtimeInvalidation', () => {
  beforeEach(() => {
    registered = null
    unsub.mockClear()
    mockOnNotification.mockClear()
  })

  it('subscribes to event.append on mount and unsubscribes on cleanup', () => {
    const { view } = setup()
    expect(mockOnNotification).toHaveBeenCalledWith('event.append', expect.any(Function))
    view.unmount()
    expect(unsub).toHaveBeenCalled()
  })

  it('does not subscribe when disabled', () => {
    setup(false)
    expect(mockOnNotification).not.toHaveBeenCalled()
  })

  it('invalidates task roots on a task.* event', () => {
    const { invalidate } = setup()
    emit('task.created')
    const keys = invalidatedKeys(invalidate)
    expect(keys).toContainEqual([...qk.tasksBoardAll])
    expect(keys).toContainEqual([...qk.taskGetAll])
  })

  it('invalidates decision roots on a decision.logged event', () => {
    const { invalidate } = setup()
    emit('decision.logged')
    const keys = invalidatedKeys(invalidate)
    expect(keys).toContainEqual([...qk.decisionsAll])
    expect(keys).toContainEqual([...qk.decisionStatsRoot])
  })

  it('invalidates missions (incl. rollup) on a kr.* event', () => {
    const { invalidate } = setup()
    emit('kr.progress_updated')
    const keys = invalidatedKeys(invalidate)
    expect(keys).toContainEqual([...qk.keyResultsAll])
    expect(keys).toContainEqual([...qk.missionsAll])
  })

  it('invalidates member root on a space.member.* event', () => {
    const { invalidate } = setup()
    emit('space.member.registered')
    expect(invalidatedKeys(invalidate)).toContainEqual([...qk.projectMembersAll])
  })

  it('ignores events with no matching prefix', () => {
    const { invalidate } = setup()
    emit('something.unknown')
    expect(invalidate).not.toHaveBeenCalled()
  })
})
