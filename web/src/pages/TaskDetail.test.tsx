import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { createTestQueryClient } from '../test/test-utils'

const mockUseTask = vi.fn()
const mockUseRecentDecisions = vi.fn()
const mockUseKeyResult = vi.fn()
const mockUseProjectKRs = vi.fn()
const mockUseMission = vi.fn()

vi.mock('../hooks/useProjectTasks', () => ({
  useTask: (...args: unknown[]) => mockUseTask(...args),
}))

vi.mock('../hooks/useDecisions', () => ({
  useRecentDecisions: (...args: unknown[]) => mockUseRecentDecisions(...args),
}))

vi.mock('../hooks/useMissions', () => ({
  useKeyResult: (...args: unknown[]) => mockUseKeyResult(...args),
  useProjectKRs: (...args: unknown[]) => mockUseProjectKRs(...args),
  useMission: (...args: unknown[]) => mockUseMission(...args),
}))

// The edit/cancel dialogs carry their own mutation hooks; stub them out so the
// page can render in isolation (they are closed by default anyway).
vi.mock('../components/task/EditTaskDialog', () => ({ default: () => null }))
vi.mock('../components/task/CancelTaskDialog', () => ({ CancelTaskDialog: () => null }))

const { default: TaskDetail } = await import('./TaskDetail')

const MISSION = { id: 'mission-1', title: 'Prepare Agen8 baseline', status: 'active' }

function renderDetail(path: string) {
  const queryClient = createTestQueryClient()
  const { hook } = memoryLocation({ path })
  return render(
    <QueryClientProvider client={queryClient}>
      <Router hook={hook}>
        <TaskDetail />
      </Router>
    </QueryClientProvider>,
  )
}

describe('TaskDetail related section', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseRecentDecisions.mockReturnValue({ data: [] })
    mockUseKeyResult.mockReturnValue({ data: undefined })
    mockUseProjectKRs.mockReturnValue({ data: new Map() })
    // useMission resolves the related mission by id (scope-independent), so both
    // the KR-linked and the directly-linked cases get the mission this way.
    mockUseMission.mockReturnValue({ data: MISSION })
  })

  it('shows the mission for a task linked directly to a mission (no KR)', async () => {
    mockUseTask.mockReturnValue({
      data: { id: 'task-m', title: 'Mission task', status: 'succeeded', missionRef: 'mission-1' },
      isLoading: false,
      isError: false,
      error: null,
    })

    renderDetail('/project/proj-1/tasks/task-m')

    expect(await screen.findByRole('heading', { name: 'Mission task' })).toBeInTheDocument()
    expect(screen.getByRole('link', { name: /prepare agen8 baseline/i })).toHaveAttribute(
      'href',
      '/project/proj-1/missions/mission-1',
    )
  })

  it('still resolves the mission through the KR for a KR-linked task', async () => {
    mockUseKeyResult.mockReturnValue({
      data: { id: 'kr-1', missionId: 'mission-1', title: 'KR one', progressPercent: 50 },
    })
    mockUseTask.mockReturnValue({
      data: { id: 'task-k', title: 'KR task', status: 'active', keyResultRef: 'kr-1' },
      isLoading: false,
      isError: false,
      error: null,
    })

    renderDetail('/project/proj-1/tasks/task-k')

    expect(await screen.findByRole('heading', { name: 'KR task' })).toBeInTheDocument()
    // Mission (via kr.missionId) and the KR itself both render as related rows.
    expect(screen.getByRole('link', { name: /prepare agen8 baseline/i })).toBeInTheDocument()
    expect(screen.getByText('KR one')).toBeInTheDocument()
  })
})
