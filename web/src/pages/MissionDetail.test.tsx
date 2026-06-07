import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { createTestQueryClient } from '../test/test-utils'

const mockRpcCall = vi.fn()
const mockUseKeyResults = vi.fn()
const mockUseProjectTasks = vi.fn()
const mockUseRecentDecisions = vi.fn()
const mockMutation = { mutateAsync: vi.fn(), isPending: false }
const mockUpdateMission = { mutateAsync: vi.fn(), isPending: false }
const mockDeleteMission = { mutateAsync: vi.fn(), isPending: false }
const mockHardDeleteMission = { mutateAsync: vi.fn(), isPending: false }

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  rpcUnwrap: async (method: string, params: unknown, field: string) => {
    const res = (await mockRpcCall(method, params)) as Record<string, unknown>
    return res[field]
  },
  rpcUnwrapList: async (method: string, params: unknown, field: string) => {
    const res = (await mockRpcCall(method, params)) as Record<string, unknown[]>
    return res[field] ?? []
  },
}))

vi.mock('../hooks/useMissions', () => ({
  useKeyResults: (...args: unknown[]) => mockUseKeyResults(...args),
  useUpdateKeyResult: () => mockMutation,
  useUpdateKRProgress: () => mockMutation,
  useDeleteKeyResult: () => mockMutation,
  useUpdateMission: () => mockUpdateMission,
  useDeleteMission: () => mockDeleteMission,
  useHardDeleteMission: () => mockHardDeleteMission,
}))

vi.mock('../hooks/useProjectTasks', () => ({
  useProjectTasks: (...args: unknown[]) => mockUseProjectTasks(...args),
}))

vi.mock('../hooks/useDecisions', () => ({
  useRecentDecisions: (...args: unknown[]) => mockUseRecentDecisions(...args),
}))

vi.mock('sonner', () => ({
  toast: {
    success: vi.fn(),
    error: vi.fn(),
  },
}))

const { default: MissionDetail } = await import('./MissionDetail')

const MISSION = {
  id: 'mission-1',
  projectId: 'proj-1',
  title: 'Stabilize public baseline',
  description: 'Make the release readable and verifiable.',
  status: 'active',
  startDate: '2026-06-06T00:00:00Z',
  endDate: '',
  createdAt: '2026-06-06T10:00:00Z',
  updatedAt: '2026-06-06T10:00:00Z',
}

const KEY_RESULTS = [
  {
    id: 'kr-1',
    missionId: 'mission-1',
    title: 'Public setup verified',
    description: 'Setup path works from zero state.',
    measurementType: 'percentage',
    direction: 'increase',
    targetValue: 100,
    currentValue: 75,
    progressPercent: 75,
    status: 'on_track',
    unit: '%',
    baseline: 0,
  },
]

function renderDetail(path = '/project/proj-1/missions/mission-1') {
  const queryClient = createTestQueryClient()
  const { hook } = memoryLocation({ path })
  return render(
    <QueryClientProvider client={queryClient}>
      <Router hook={hook}>
        <MissionDetail />
      </Router>
    </QueryClientProvider>,
  )
}

describe('MissionDetail page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUpdateMission.mutateAsync.mockResolvedValue({ mission: MISSION })
    mockDeleteMission.mutateAsync.mockResolvedValue({ mission: MISSION })
    mockHardDeleteMission.mutateAsync.mockResolvedValue({ mission: MISSION })
    mockRpcCall.mockResolvedValue({ mission: MISSION })
    mockUseKeyResults.mockReturnValue({ data: KEY_RESULTS, isLoading: false, isError: false, error: null })
    mockUseProjectTasks.mockReturnValue({
      data: [
        { id: 'task-1', title: 'Verify setup docs', keyResultRef: 'kr-1' },
        { id: 'task-unrelated', title: 'Unrelated cleanup', keyResultRef: 'kr-other' },
      ],
    })
    mockUseRecentDecisions.mockReturnValue({
      data: [
        { id: 'dec-1', title: 'Keep setup docs strict', missionRef: 'mission-1', keyResultRef: '', confidence: 0.9 },
        { id: 'dec-2', title: 'Track setup through KR', missionRef: '', keyResultRef: 'kr-1', confidence: 0.7 },
        { id: 'dec-unrelated', title: 'Unrelated decision', missionRef: 'mission-other', keyResultRef: '', confidence: 0.8 },
      ],
    })
  })

  it('renders related task and decision links for the mission and its key results', async () => {
    renderDetail()

    expect(await screen.findByRole('heading', { name: 'Stabilize public baseline' })).toBeInTheDocument()

    expect(screen.getByRole('link', { name: /verify setup docs/i })).toHaveAttribute(
      'href',
      '/project/proj-1/tasks/task-1',
    )
    expect(screen.getByRole('link', { name: /keep setup docs strict/i })).toHaveAttribute(
      'href',
      '/project/proj-1/decisions/dec-1',
    )
    expect(screen.getByRole('link', { name: /track setup through kr/i })).toHaveAttribute(
      'href',
      '/project/proj-1/decisions/dec-2',
    )

    expect(screen.queryByText('Unrelated cleanup')).not.toBeInTheDocument()
    expect(screen.queryByText('Unrelated decision')).not.toBeInTheDocument()
  })

  it('opens the edit dialog and saves through mission.update', async () => {
    const user = userEvent.setup()
    renderDetail()

    await screen.findByRole('heading', { name: 'Stabilize public baseline' })

    const actions = screen.getByRole('group', { name: /mission actions/i })
    await user.click(within(actions).getByRole('button', { name: /^edit$/i }))

    const dialog = await screen.findByRole('dialog')
    expect(within(dialog).getByText('Edit mission')).toBeInTheDocument()

    await user.click(within(dialog).getByRole('button', { name: /save changes/i }))

    await waitFor(() => {
      expect(mockUpdateMission.mutateAsync).toHaveBeenCalledWith(
        expect.objectContaining({ missionId: 'mission-1', title: 'Stabilize public baseline' }),
      )
    })
  })

  it('archives the mission through the confirm dialog wired to mission.delete', async () => {
    const user = userEvent.setup()
    renderDetail()

    await screen.findByRole('heading', { name: 'Stabilize public baseline' })

    // The header trigger opens the confirm; archive (soft delete) lives inside it.
    const actions = screen.getByRole('group', { name: /mission actions/i })
    await user.click(within(actions).getByRole('button', { name: /^delete$/i }))
    const confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: /^archive$/i }))

    await waitFor(() => {
      expect(mockDeleteMission.mutateAsync).toHaveBeenCalledWith({ missionId: 'mission-1' })
    })
    expect(mockHardDeleteMission.mutateAsync).not.toHaveBeenCalled()
  })

  it('permanently deletes the mission through the confirm dialog wired to mission.purge', async () => {
    const user = userEvent.setup()
    renderDetail()

    await screen.findByRole('heading', { name: 'Stabilize public baseline' })

    // The same confirm offers an explicit, irreversible hard delete.
    const actions = screen.getByRole('group', { name: /mission actions/i })
    await user.click(within(actions).getByRole('button', { name: /^delete$/i }))
    const confirm = await screen.findByRole('alertdialog')
    await user.click(within(confirm).getByRole('button', { name: /delete permanently/i }))

    await waitFor(() => {
      expect(mockHardDeleteMission.mutateAsync).toHaveBeenCalledWith({ missionId: 'mission-1' })
    })
    expect(mockDeleteMission.mutateAsync).not.toHaveBeenCalled()
  })

  it('exposes lifecycle transitions for an active mission', async () => {
    renderDetail()

    await screen.findByRole('heading', { name: 'Stabilize public baseline' })

    // Active missions can be paused or completed; those controls must be reachable.
    expect(screen.getByRole('button', { name: /pause/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /complete/i })).toBeInTheDocument()
  })
})
