import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook, waitFor } from '@testing-library/react'
import { createWrapper } from '../../test/test-utils'
import type { ContextLink } from '../../lib/types'

const mockRpcCall = vi.fn()

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

const { useContextLinkEdges } = await import('./useContextLinkEdges')

// ── Helpers ────────────────────────────────────────────────────────────────────

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

// ── Node ID mapping tests (toNodeId / nodeToEntity round-trip) ─────────────────

describe('useContextLinkEdges — node ID mapping', () => {
  beforeEach(() => {
    mockRpcCall.mockReset().mockResolvedValue({ contextLinks: [] })
  })

  it('emits edge with correct source ID for task node (task: prefix)', async () => {
    const link = makeLink({
      sourceType: 'task', sourceId: 'task-aaa',
      targetType: 'key_result', targetId: 'kr-bbb',
    })
    mockRpcCall.mockResolvedValue({ contextLinks: [link] })

    const nodeIds = ['task:task-aaa', 'kr-bbb']

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinkEdges(['kr-bbb'], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.edges).toHaveLength(1)
    const edge = result.current.edges[0]
    expect(edge.source).toBe('task:task-aaa')
    expect(edge.target).toBe('kr-bbb')
    expect(edge.type).toBe('contextEdge')
    expect(edge.data).toMatchObject({ edgeType: 'serves' })
  })

  it('emits edge with correct source ID for decision node (decision: prefix)', async () => {
    const link = makeLink({
      sourceType: 'decision', sourceId: 'dec-aaa',
      targetType: 'key_result', targetId: 'kr-bbb',
      edgeType: 'serves',
    })
    mockRpcCall.mockResolvedValue({ contextLinks: [link] })

    const nodeIds = ['decision:dec-aaa', 'kr-bbb']

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinkEdges(['kr-bbb'], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))

    expect(result.current.edges).toHaveLength(1)
    expect(result.current.edges[0].source).toBe('decision:dec-aaa')
    expect(result.current.edges[0].target).toBe('kr-bbb')
  })
})

// ── Edge filtering tests ───────────────────────────────────────────────────────

describe('useContextLinkEdges — edge filtering', () => {
  beforeEach(() => {
    mockRpcCall.mockReset()
  })

  it('drops edges where source node is not on the graph', async () => {
    const link = makeLink({
      sourceType: 'task', sourceId: 'task-absent',
      targetType: 'key_result', targetId: 'kr-bbb',
    })
    mockRpcCall.mockResolvedValue({ contextLinks: [link] })

    // Only target node is present; source is missing
    const nodeIds = ['kr-bbb']

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinkEdges(['kr-bbb'], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.edges).toHaveLength(0)
  })

  it('drops edges where target node is not on the graph', async () => {
    const link = makeLink({
      sourceType: 'task', sourceId: 'task-aaa',
      targetType: 'key_result', targetId: 'kr-absent',
    })
    mockRpcCall.mockResolvedValue({ contextLinks: [link] })

    // Only source node is present; target is missing
    const nodeIds = ['task:task-aaa']

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinkEdges([], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.edges).toHaveLength(0)
  })

  it('emits edge id in cl: prefix format', async () => {
    const link = makeLink({ id: 'abc123' })
    mockRpcCall.mockResolvedValue({ contextLinks: [link] })

    const nodeIds = ['task:task-aaa', 'kr-bbb']

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinkEdges(['kr-bbb'], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() => expect(result.current.isLoading).toBe(false))
    expect(result.current.edges[0].id).toBe('cl:abc123')
  })

  it('returns empty edges when no nodes provided', () => {
    mockRpcCall.mockResolvedValue({ contextLinks: [] })

    const { Wrapper } = createWrapper()
    const { result } = renderHook(
      () => useContextLinkEdges([], [], []),
      { wrapper: Wrapper },
    )

    expect(result.current.edges).toHaveLength(0)
  })
})

// ── Leaf source query dispatch ─────────────────────────────────────────────────
// This tests that leaf node types trigger listBySource queries with the
// correctly-stripped entity ID (no prefix in the RPC call).

describe('useContextLinkEdges — leaf source queries', () => {
  beforeEach(() => {
    mockRpcCall.mockReset().mockResolvedValue({ contextLinks: [] })
  })

  it('calls listBySource with raw task id (no prefix) for task nodes', async () => {
    const nodeIds = ['task:task-aaa']

    const { Wrapper } = createWrapper()
    renderHook(
      () => useContextLinkEdges([], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() =>
      expect(mockRpcCall).toHaveBeenCalledWith('graph.linksBySource', {
        sourceType: 'task',
        sourceId: 'task-aaa',   // no "task:" prefix — raw DB id
      }),
    )
  })

  it('calls listBySource with raw decision id for decision nodes', async () => {
    const nodeIds = ['decision:dec-bbb']

    const { Wrapper } = createWrapper()
    renderHook(
      () => useContextLinkEdges([], [], nodeIds),
      { wrapper: Wrapper },
    )

    await waitFor(() =>
      expect(mockRpcCall).toHaveBeenCalledWith('graph.linksBySource', {
        sourceType: 'decision',
        sourceId: 'dec-bbb',
      }),
    )
  })

  it('does NOT call listBySource for mission or keyResult nodes', async () => {
    const nodeIds = ['mission-1', 'kr-1']

    const { Wrapper } = createWrapper()
    renderHook(
      () => useContextLinkEdges([], [], nodeIds),
      { wrapper: Wrapper },
    )

    // Allow a tick for any async calls
    await new Promise(r => setTimeout(r, 50))

    // No listBySource calls — missions and KRs are targets, not sources
    expect(mockRpcCall).not.toHaveBeenCalledWith(
      'graph.linksBySource',
      expect.anything(),
    )
  })

  it('does not re-query when rerendered with the same node IDs in a new array', async () => {
    const { Wrapper } = createWrapper()
    const { rerender } = renderHook(
      ({ nodeIds }) => useContextLinkEdges(['kr-bbb'], [], nodeIds),
      {
        wrapper: Wrapper,
        initialProps: { nodeIds: ['task:task-aaa', 'kr-bbb'] },
      },
    )

    await waitFor(() => expect(mockRpcCall).toHaveBeenCalled())

    const initialCalls = mockRpcCall.mock.calls.length

    rerender({ nodeIds: ['task:task-aaa', 'kr-bbb'] })

    await new Promise(r => setTimeout(r, 50))
    expect(mockRpcCall.mock.calls).toHaveLength(initialCalls)
  })
})
