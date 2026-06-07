import { beforeEach, describe, expect, it, vi } from 'vitest'
import { renderHook } from '@testing-library/react'
import { useLeafNodes } from './useLeafNodes'

vi.mock('../../hooks/useProjectTasks', () => ({ useProjectTasks: vi.fn() }))
vi.mock('../../hooks/useDecisions', () => ({ useRecentDecisions: vi.fn() }))

import { useProjectTasks } from '../../hooks/useProjectTasks'
import { useRecentDecisions } from '../../hooks/useDecisions'

const mockUseProjectTasks = vi.mocked(useProjectTasks)
const mockUseRecentDecisions = vi.mocked(useRecentDecisions)

beforeEach(() => {
  mockUseProjectTasks.mockReset()
  mockUseRecentDecisions.mockReset()
  // No decisions in these cases — task topology is what we're exercising.
  mockUseRecentDecisions.mockReturnValue({ data: [], isLoading: false } as never)
})

describe('useLeafNodes task topology', () => {
  it('renders a mission-linked task (no KR) as a node with a mission→task edge', () => {
    mockUseProjectTasks.mockReturnValue({
      data: [{ id: 'task-m', missionRef: 'mis-1', status: 'pending', title: 'Mission task' }],
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useLeafNodes('playground'))

    expect(result.current.nodes.map((n) => n.id)).toContain('task:task-m')
    expect(result.current.edges).toContainEqual(
      expect.objectContaining({ source: 'mis-1', target: 'task:task-m' }),
    )
  })

  it('still renders a KR-linked task with a kr→task edge', () => {
    mockUseProjectTasks.mockReturnValue({
      data: [{ id: 'task-k', keyResultRef: 'kr-1', status: 'pending', title: 'KR task' }],
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useLeafNodes('playground'))

    expect(result.current.nodes.map((n) => n.id)).toContain('task:task-k')
    expect(result.current.edges).toContainEqual(
      expect.objectContaining({ source: 'kr-1', target: 'task:task-k' }),
    )
  })

  it('prefers the KR parent when a task has both refs', () => {
    mockUseProjectTasks.mockReturnValue({
      data: [
        { id: 'task-b', keyResultRef: 'kr-1', missionRef: 'mis-1', status: 'pending', title: 'Both' },
      ],
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useLeafNodes('playground'))

    expect(result.current.edges).toContainEqual(
      expect.objectContaining({ source: 'kr-1', target: 'task:task-b' }),
    )
    expect(result.current.edges.some((e) => e.source === 'mis-1')).toBe(false)
  })

  it('omits an orphan task with neither ref from the graph', () => {
    mockUseProjectTasks.mockReturnValue({
      data: [{ id: 'task-o', status: 'pending', title: 'Orphan' }],
      isLoading: false,
    } as never)

    const { result } = renderHook(() => useLeafNodes('playground'))

    expect(result.current.nodes.map((n) => n.id)).not.toContain('task:task-o')
  })
})
