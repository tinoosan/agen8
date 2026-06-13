import { describe, it, expect, vi } from 'vitest'
import { renderHook } from '@testing-library/react'

// Mock dependencies used by useNavigation
vi.mock('wouter', () => ({
  useLocation: () => ['/project/proj-1/dashboard', vi.fn()],
}))

vi.mock('../hooks/useProjects', () => ({
  useProjects: () => ({
    data: [
      { id: 'proj-1', root: '/repo', title: 'Project 1', status: 'open' },
    ],
    isLoading: false,
  }),
}))

const { useNavigation, tasksPanelLink, filteredTasksLink, decisionDetailLink } = await import('./routing')

describe('useNavigation', () => {
  it('parses projectId from URL', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.projectId).toBe('proj-1')
  })

  it('resolves focusedProjectRoot from project data', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.focusedProjectRoot).toBe('/repo')
  })

  it('returns setter functions', () => {
    const { result } = renderHook(() => useNavigation())
    expect(typeof result.current.setFocusedProjectRoot).toBe('function')
    expect(typeof result.current.setActiveView).toBe('function')
  })

  it('returns projectLoading false when project is found', () => {
    const { result } = renderHook(() => useNavigation())
    expect(result.current.projectLoading).toBe(false)
  })

  it('resolves the tasks list to its dedicated page', () => {
    expect(tasksPanelLink('proj-1')).toBe('/project/proj-1/tasks')
  })

  it('builds a status-filtered tasks link on the Tasks page', () => {
    expect(filteredTasksLink('proj-1', 'active')).toBe('/project/proj-1/tasks?status=active')
  })

  it('omits the status query parameter for the default (all) filter', () => {
    expect(filteredTasksLink('proj-1', 'all')).toBe('/project/proj-1/tasks')
  })

  it('builds routed decision detail links', () => {
    expect(decisionDetailLink('proj-1', 'dec-123')).toBe('/project/proj-1/decisions/dec-123')
  })
})
