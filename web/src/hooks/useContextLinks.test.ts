import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '../test/test-utils'
import type { ContextLink } from '../lib/types'

const mockRpcCall = vi.fn()

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

const { useContextLinks } = await import('./useContextLinks')

function makeLink(overrides: Partial<ContextLink> = {}): ContextLink {
  return {
    id: 'cl-1',
    sourceType: 'task',
    sourceId: 'task-aaa',
    targetType: 'key_result',
    targetId: 'kr-bbb',
    edgeType: 'serves',
    confidence: 1.0,
    createdAt: '2026-01-01T00:00:00Z',
    ...overrides,
  }
}

describe('useContextLinks', () => {
  beforeEach(() => {
    mockRpcCall.mockReset()
  })

  it('does not fetch when all arrays are empty', () => {
    const { Wrapper } = createWrapper()
    renderHook(() => useContextLinks([], [], []), { wrapper: Wrapper })
    expect(mockRpcCall).not.toHaveBeenCalled()
  })

  it('calls listByTarget for each KR id', async () => {
    mockRpcCall.mockResolvedValue({ contextLinks: [] })

    const { Wrapper } = createWrapper()
    renderHook(
      () => useContextLinks(['kr-1', 'kr-2'], [], []),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(2))

    expect(mockRpcCall).toHaveBeenCalledWith('graph.linksByTarget', {
      targetType: 'key_result',
      targetId: 'kr-1',
    })
    expect(mockRpcCall).toHaveBeenCalledWith('graph.linksByTarget', {
      targetType: 'key_result',
      targetId: 'kr-2',
    })
  })

  it('calls listByTarget for each mission id', async () => {
    mockRpcCall.mockResolvedValue({ contextLinks: [] })

    const { Wrapper } = createWrapper()
    renderHook(
      () => useContextLinks([], ['mission-1'], []),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(1))

    expect(mockRpcCall).toHaveBeenCalledWith('graph.linksByTarget', {
      targetType: 'mission',
      targetId: 'mission-1',
    })
  })

  it('calls listBySource for each leaf entity', async () => {
    mockRpcCall.mockResolvedValue({ contextLinks: [] })

    const { Wrapper } = createWrapper()
    renderHook(
      () =>
        useContextLinks([], [], [
          { entityType: 'task', entityId: 'task-aaa' },
          { entityType: 'decision', entityId: 'dec-bbb' },
        ]),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalledTimes(2))

    expect(mockRpcCall).toHaveBeenCalledWith('graph.linksBySource', {
      sourceType: 'task',
      sourceId: 'task-aaa',
    })
    expect(mockRpcCall).toHaveBeenCalledWith('graph.linksBySource', {
      sourceType: 'decision',
      sourceId: 'dec-bbb',
    })
  })

  it('deduplicates links returned from multiple queries', async () => {
    const link = makeLink({ id: 'cl-dup' })
    mockRpcCall.mockResolvedValue({ contextLinks: [link] })

    const { Wrapper } = createWrapper()
    // Two queries will both return the same link — dedup expected
    const { result } = renderHook(
      () => useContextLinks(['kr-1', 'kr-2'], [], []),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.contextLinks).toHaveLength(1)
    expect(result.current.contextLinks[0].id).toBe('cl-dup')
  })

  it('returns all unique links from mixed target + source queries', async () => {
    const targetLink = makeLink({ id: 'cl-target', sourceType: 'task', sourceId: 'task-aaa', targetType: 'key_result', targetId: 'kr-1', edgeType: 'serves' })
    const sourceLink = makeLink({ id: 'cl-source', sourceType: 'decision', sourceId: 'dec-bbb', targetType: 'task', targetId: 'task-aaa', edgeType: 'made_during' })

    mockRpcCall
      .mockResolvedValueOnce({ contextLinks: [targetLink] })      // listByTarget kr-1
      .mockResolvedValueOnce({ contextLinks: [sourceLink] })      // listBySource task-aaa

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () =>
        useContextLinks(
          ['kr-1'],
          [],
          [{ entityType: 'task', entityId: 'task-aaa' }],
        ),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.contextLinks).toHaveLength(2)
    const ids = result.current.contextLinks.map(l => l.id).sort()
    expect(ids).toEqual(['cl-source', 'cl-target'])
  })

  it('isLoading is true while queries are pending', () => {
    // Never resolves — stays pending
    mockRpcCall.mockReturnValue(new Promise(() => {}))

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinks(['kr-1'], [], []),
      { wrapper: Wrapper },
    )

    expect(result.current.isLoading).toBe(true)
  })
})
