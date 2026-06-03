import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

// Mock dependencies used by useNavigation
vi.mock('wouter', () => ({
  useLocation: () => ['/project/proj-1/space/space-abc', vi.fn()],
}))

vi.mock('../hooks/useProjects', () => ({
  useProjects: () => ({
    data: [
      { id: 'proj-1', root: '/repo', title: 'Project 1', status: 'open' },
    ],
    isLoading: false,
  }),
}))

const { useNavigation, boardTaskLink } = await import('./routing')

describe('useNavigation', () => {
  it('parses projectId from URL', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.projectId).toBe('proj-1')
  })

  it('parses spaceId from URL', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.focusedSpaceId).toBe('space-abc')
  })

  it('resolves focusedProjectRoot from project data', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.focusedProjectRoot).toBe('/repo')
  })

  it('returns setter functions', () => {
    const { result } = renderHook(() => useNavigation())
    expect(typeof result.current.setFocusedProjectRoot).toBe('function')
    expect(typeof result.current.setFocusedSpaceId).toBe('function')
    expect(typeof result.current.setActiveView).toBe('function')
  })

  it('returns projectLoading false when project is found', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.projectLoading).toBe(false)
  })

  it('builds board task links for the space board tab', () => {
    expect(boardTaskLink('proj-1', 'space-abc', 'task-123')).toBe('/project/proj-1/space/space-abc?tab=board&task=task-123')
  })

  it('omits the task query parameter when the task id is empty', () => {
    expect(boardTaskLink('proj-1', 'space-abc', '')).toBe('/project/proj-1/space/space-abc?tab=board')
  })
})
