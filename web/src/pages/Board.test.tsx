import { describe, it, expect, beforeEach, afterEach, vi } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Task } from '../lib/types'

const mockUseSearch = vi.fn(() => 'space=th-b')

const sampleTask: Task = {
  id: 'task-1',
  title: 'Do work',
  goal: 'Do work',
  status: 'pending',
  spaceId: 'th-b',
  assignedRole: 'worker',
  createdAt: new Date().toISOString(),
}

vi.mock('wouter', async (importOriginal) => {
  const mod = await importOriginal<typeof import('wouter')>()
  return {
    ...mod,
    useSearch: () => mockUseSearch(),
  }
})

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({ focusedProjectRoot: '/repo', projectId: 'proj-1' }),
}))

const mockUseProjectSpaces = vi.fn()
vi.mock('../hooks/useProjectSpaces', () => ({
  useProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
}))

const mockUseProjectTasks = vi.fn(() => ({ data: [] as Task[], isLoading: false, isError: false }))
vi.mock('../hooks/useProjectTasks', () => ({
  useProjectTasks: () => mockUseProjectTasks(),
  useProjectTasksSSE: () => {},
}))

const mockUsePendingOpActions = vi.fn(() => ({ data: [], isLoading: false, isError: false }))
vi.mock('../hooks/useOpActions', () => ({
  usePendingOpActions: (...args: unknown[]) => mockUsePendingOpActions(...args),
}))

vi.mock('../components/dashboard/OpActionDetailPanel', () => ({
  default: ({ actionId }: { actionId: string | null }) => (
    actionId ? <div>Operator action panel: {actionId}</div> : null
  ),
}))

const { default: Board } = await import('./Board')

function renderBoard() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Board />
    </QueryClientProvider>,
  )
}

describe('Board', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseSearch.mockReturnValue('space=th-b')
    mockUseProjectTasks.mockReturnValue({ data: [sampleTask], isLoading: false, isError: false })
    mockUsePendingOpActions.mockReturnValue({ data: [], isLoading: false, isError: false })
    mockUseProjectSpaces.mockReturnValue({
      data: [
        { spaceId: 'th-a', spaceName: 'Alpha' },
        { spaceId: 'th-b', spaceName: 'Beta' },
      ],
      isLoading: false,
    })
  })

  it('applies space filter once from ?space= when the space exists', async () => {
    renderBoard()
    await waitFor(() => {
      expect(screen.getByText('Beta')).toBeInTheDocument()
    })
  })

  it('resolves ?space= from space name to canonical space id', async () => {
    mockUseSearch.mockReturnValue('space=beta')
    renderBoard()
    await waitFor(() => {
      expect(screen.getByText('Beta')).toBeInTheDocument()
    })
  })

  it('does not show awaiting-operator badge for dependency-blocked tasks', async () => {
    mockUseSearch.mockReturnValue('')
    mockUseProjectTasks.mockReturnValue({
      data: [{
        ...sampleTask,
        status: 'blocked',
        metadata: { blockedBy: [{ kind: 'task', id: 'task-prereq' }] },
      }],
      isLoading: false,
      isError: false,
    })

    renderBoard()

    await waitFor(() => {
      expect(screen.getByText('Do work')).toBeInTheDocument()
    })
    expect(screen.queryByText('Awaiting Operator')).not.toBeInTheDocument()
  })

  it('shows awaiting-operator badge for operator-action blockers and opens the panel', async () => {
    mockUseSearch.mockReturnValue('')
    mockUseProjectTasks.mockReturnValue({
      data: [{
        ...sampleTask,
        status: 'blocked',
        metadata: {
          blockedBy: [{ kind: 'operator_action', id: 'oa-1', reason: 'Need operator review' }],
          blockReason: 'Need operator review',
        },
      }],
      isLoading: false,
      isError: false,
    })
    mockUsePendingOpActions.mockReturnValue({
      data: [{
        id: 'oa-1',
        projectId: 'proj-1',
        blocking: true,
        source: 'agent',
        category: 'general',
        urgency: 'high',
        title: 'Review supplier agreement',
        description: 'Operator needs to review the contract terms.',
        requiresVerification: false,
        status: 'pending',
        createdAt: new Date().toISOString(),
      }],
      isLoading: false,
      isError: false,
    })

    renderBoard()

    const badgeButton = await screen.findByRole('button', { name: 'Awaiting Operator (high)' })
    expect(screen.getByText('Review supplier agreement')).toBeInTheDocument()

    fireEvent.click(badgeButton)

    expect(screen.getByText('Operator action panel: oa-1')).toBeInTheDocument()
  })

  describe('column limits', () => {
    afterEach(() => {
      localStorage.removeItem('agen8.board.wip.proj-1')
    })

    it('shows plain count badge when no column limit is configured', async () => {
      renderBoard()
      await waitFor(() => {
        // Column header count badge shows just the number, no "1/N" format
        expect(screen.queryByText(/1\/\d+/)).not.toBeInTheDocument()
        // The board renders (has the Board heading)
        expect(screen.getByRole('heading', { name: 'Board' })).toBeInTheDocument()
      })
    })

    it('shows count/limit in column header when column limit is configured', async () => {
      localStorage.setItem('agen8.board.wip.proj-1', JSON.stringify({ backlog: 5 }))
      renderBoard()
      await waitFor(() => {
        expect(screen.getByText('1/5')).toBeInTheDocument()
      })
    })

    it('applies over-limit indicator when count exceeds column limit', async () => {
      const extraTasks = Array.from({ length: 3 }, (_, i) => ({
        ...sampleTask,
        id: `task-${i + 2}`,
        status: 'pending',
      }))
      mockUseProjectTasks.mockReturnValue({
        data: [sampleTask, ...extraTasks],
        isLoading: false,
        isError: false,
      })
      localStorage.setItem('agen8.board.wip.proj-1', JSON.stringify({ backlog: 2 }))
      renderBoard()
      await waitFor(() => {
        // 4 tasks, limit 2: shows "4/2"
        expect(screen.getByText('4/2')).toBeInTheDocument()
      })
    })
  })

  describe('task cycle time', () => {
    it('shows duration badge for completed tasks', async () => {
      const completedTask = {
        ...sampleTask,
        id: 'task-done',
        status: 'succeeded',
        createdAt: '2026-04-05T10:00:00Z',
        completedAt: '2026-04-05T10:30:00Z',
      }
      mockUseProjectTasks.mockReturnValue({ data: [completedTask], isLoading: false, isError: false })
      renderBoard()
      await waitFor(() => {
        expect(screen.getByText('30m')).toBeInTheDocument()
      })
    })
  })
})
