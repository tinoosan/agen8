import { describe, it, expect, vi, beforeEach } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useLeafNodes } from './useLeafNodes'

vi.mock('../../hooks/useProjectSpaces', () => ({ useProjectSpaces: vi.fn() }))
vi.mock('../../hooks/useProjectTasks', () => ({ useProjectTasks: vi.fn() }))
vi.mock('../../hooks/useDecisions', () => ({ useRecentDecisions: vi.fn() }))
vi.mock('../../hooks/useOpActions', () => ({ useStrategyMapOpActions: vi.fn() }))
vi.mock('../../hooks/useEscalations', () => ({ useAllEscalations: vi.fn() }))

import { useProjectSpaces } from '../../hooks/useProjectSpaces'
import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'
import { useStrategyMapOpActions } from '../../hooks/useOpActions'
import { useAllEscalations } from '../../hooks/useEscalations'

const mockUseProjectSpaces = vi.mocked(useProjectSpaces)
const mockUseProjectTasks = vi.mocked(useProjectTasks)
const mockUseRecentDecisions = vi.mocked(useRecentDecisions)
const mockUseStrategyMapOpActions = vi.mocked(useStrategyMapOpActions)
const mockUseAllEscalations = vi.mocked(useAllEscalations)

beforeEach(() => {
  mockUseProjectSpaces.mockReturnValue({ data: [], isLoading: false } as never)
  mockUseProjectTasks.mockReturnValue({ data: [], isLoading: false } as never)
  mockUseRecentDecisions.mockReturnValue({ data: [], isLoading: false } as never)
  mockUseStrategyMapOpActions.mockReturnValue({ data: [], isLoading: false } as never)
  mockUseAllEscalations.mockReturnValue({ data: [], isLoading: false } as never)
})

describe('useLeafNodes', () => {
  it('loads spaces including deleted instances for cross-space task visibility', () => {
    renderHook(() => useLeafNodes('proj-1', '/tmp/project'))
    expect(mockUseProjectSpaces).toHaveBeenCalledWith('proj-1', {
      refetchInterval: 30_000,
      includeDeleted: true,
    })
  })

  it('includes completed and active OA nodes in the strategy map', () => {
    mockUseStrategyMapOpActions.mockReturnValue({
      data: [
        { id: 'oa-1', title: 'Done OA', status: 'completed', taskRef: 'task-1', blocking: false, createdAt: '2026-01-01T00:00:00Z' },
        { id: 'oa-2', title: 'Active OA', status: 'in_progress', keyResultRef: 'kr-2', blocking: false, createdAt: '2026-01-02T00:00:00Z' },
      ],
      isLoading: false,
    } as never)
    mockUseProjectTasks.mockReturnValue({
      data: [{ id: 'task-1', keyResultRef: 'kr-1' }],
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useLeafNodes('proj-1', '/tmp/project'))

    const nodeIds = result.current.nodes.map((n) => n.id)
    expect(nodeIds).toContain('oa:oa-1')
    expect(nodeIds).toContain('oa:oa-2')
  })

  it('prefers task parent for OA edge routing when taskRef is in graph', () => {
    mockUseStrategyMapOpActions.mockReturnValue({
      data: [
        {
          id: 'oa-1',
          title: 'Routed OA',
          status: 'completed',
          taskRef: 'task-1',
          keyResultRef: 'kr-fallback',
          blocking: false,
          createdAt: '2026-01-01T00:00:00Z',
        },
      ],
      isLoading: false,
    } as never)
    mockUseProjectTasks.mockReturnValue({
      data: [{ id: 'task-1', keyResultRef: 'kr-1' }],
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useLeafNodes('proj-1', '/tmp/project'))
    const oaEdge = result.current.edges.find((e) => e.id.startsWith('oa:oa-1-->'))
    expect(oaEdge).toBeDefined()
    expect(oaEdge?.source).toBe('task:task-1')
  })
})
