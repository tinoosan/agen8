import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { createTestQueryClient } from '../test/test-utils'

const mockUseTask = vi.fn()
const mockMutateAsync = vi.fn()
const mockAssignAsync = vi.fn()
const mockUseRecentDecisions = vi.fn()
const mockUseKeyResult = vi.fn()
const mockUseProjectKRs = vi.fn()
const mockUseMission = vi.fn()

vi.mock('../hooks/useProjectTasks', () => ({
  useTask: (...args: unknown[]) => mockUseTask(...args),
  // Editing is inline on the page now, so the page itself drives task.update
  // and (on assignee change) task.assign.
  useUpdateTask: () => ({ mutateAsync: mockMutateAsync, isPending: false }),
  useAssignTask: () => ({ mutateAsync: mockAssignAsync, isPending: false }),
}))

vi.mock('../hooks/useProjectMembers', () => ({
  useProjectMembers: () => ({
    data: [
      { id: 'member-1', displayName: 'Forge (Full Stack Engineer)', lifecycleState: 'active' },
      { id: 'member-2', displayName: 'Nova (Backend Engineer)', lifecycleState: 'active' },
    ],
    isLoading: false,
  }),
}))

vi.mock('../hooks/useDecisions', () => ({
  useRecentDecisions: (...args: unknown[]) => mockUseRecentDecisions(...args),
}))

vi.mock('../hooks/useMissions', () => ({
  useKeyResult: (...args: unknown[]) => mockUseKeyResult(...args),
  useProjectKRs: (...args: unknown[]) => mockUseProjectKRs(...args),
  useMission: (...args: unknown[]) => mockUseMission(...args),
}))

// The cancel dialog carries its own mutation hook; stub it out so the page can
// render in isolation (it is closed by default anyway).
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
    mockMutateAsync.mockResolvedValue({ task: {} })
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

describe('TaskDetail inline editing', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockMutateAsync.mockResolvedValue({ task: {} })
    mockUseRecentDecisions.mockReturnValue({ data: [] })
    mockUseKeyResult.mockReturnValue({ data: undefined })
    mockUseProjectKRs.mockReturnValue({ data: new Map() })
    mockUseMission.mockReturnValue({ data: MISSION })
    mockUseTask.mockReturnValue({
      data: {
        id: 'task-e',
        title: 'Editable task',
        description: 'Do the thing',
        status: 'active',
        missionRef: 'mission-1',
      },
      isLoading: false,
      isError: false,
      error: null,
    })
  })

  it('edits the title in place and saves via task.update', async () => {
    renderDetail('/project/proj-1/tasks/task-e')

    // Read mode: title is a heading, not an input.
    expect(await screen.findByRole('heading', { name: 'Editable task' })).toBeInTheDocument()

    // Enter edit mode — the title becomes an input seeded with the current value.
    fireEvent.click(screen.getByRole('button', { name: /edit/i }))
    const titleInput = screen.getByRole('textbox', { name: 'Task title' })
    expect(titleInput).toHaveValue('Editable task')

    fireEvent.change(titleInput, { target: { value: 'Renamed task' } })
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))

    await waitFor(() =>
      expect(mockMutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({ taskId: 'task-e', title: 'Renamed task', description: 'Do the thing' }),
      ),
    )
  })

  it('blocks saving when the description is emptied', async () => {
    renderDetail('/project/proj-1/tasks/task-e')

    fireEvent.click(await screen.findByRole('button', { name: /edit/i }))
    const descInput = screen.getByLabelText('Goal')
    fireEvent.change(descInput, { target: { value: '   ' } })
    fireEvent.click(screen.getByRole('button', { name: /save changes/i }))

    // Required-description guard: task.update is never called for an empty body.
    await waitFor(() => expect(mockMutateAsync).not.toHaveBeenCalled())
  })

  it('offers an assignee picker in edit mode for a non-terminal task', async () => {
    renderDetail('/project/proj-1/tasks/task-e')

    fireEvent.click(await screen.findByRole('button', { name: /edit/i }))
    // The assignee picker is a labelled combobox (Radix Select trigger).
    expect(screen.getByRole('combobox', { name: 'Assignee' })).toBeInTheDocument()
  })

  it('hides the assignee picker for a terminal task (reassign is rejected by the backend)', async () => {
    mockUseTask.mockReturnValue({
      data: {
        id: 'task-done',
        title: 'Finished task',
        description: 'All done',
        status: 'succeeded',
        missionRef: 'mission-1',
      },
      isLoading: false,
      isError: false,
      error: null,
    })

    renderDetail('/project/proj-1/tasks/task-done')

    fireEvent.click(await screen.findByRole('button', { name: /edit/i }))
    // Still editable (title input present) but no assignee picker for a terminal task.
    expect(screen.getByRole('textbox', { name: 'Task title' })).toBeInTheDocument()
    expect(screen.queryByRole('combobox', { name: 'Assignee' })).not.toBeInTheDocument()
  })
})
