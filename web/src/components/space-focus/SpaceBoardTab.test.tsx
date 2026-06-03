import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { beforeEach, describe, expect, it, vi } from 'vitest'
import type { ComponentProps } from 'react'
import SpaceBoardTab from './SpaceBoardTab'
import type { Task, SpaceMember } from '../../lib/types'

const mockRpcCall = vi.fn()

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

function renderBoard(props: Partial<ComponentProps<typeof SpaceBoardTab>> = {}) {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false },
      mutations: { retry: false },
    },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <SpaceBoardTab spaceId="space-1" {...props} />
    </QueryClientProvider>,
  )
}

describe('SpaceBoardTab', () => {
  const member: SpaceMember = {
    id: 'member-1',
    spaceId: 'space-1',
    channelId: 'ch-1',
    displayName: 'backend-engineer',
    memberType: 'worker',
    lifecycleState: 'active',
    harnessKind: 'llm',
    model: 'claude-4-sonnet',
    effort: 'medium',
  }

  const task: Task = {
    id: 'task-1',
    spaceId: 'space-1',
    title: 'Write repeatable disconnect checklist',
    description: 'Write checklist',
    status: 'pending',
    assignedTo: 'member-1',
    createdAt: '2026-04-24T10:00:00Z',
  }

  beforeEach(() => {
    mockRpcCall.mockReset()
    mockRpcCall.mockImplementation((method: string, _params: Record<string, unknown>) => {
      if (method === 'task.list') {
        return Promise.resolve({ tasks: [task] })
      }
      if (method === 'space.member.list') {
        return Promise.resolve({ members: [member] })
      }
      if (method === 'task.create' || method === 'task.update' || method === 'task.cancel') {
        return Promise.resolve({ task })
      }
      return Promise.reject(new Error(`unexpected method ${method}`))
    })
  })

  it('renders tasks as cards with the assignee on the card', async () => {
    renderBoard()

    // Task title appears as a card
    await screen.findByText('Write repeatable disconnect checklist')

    // Member moves onto the card as an assignee chip
    await screen.findByText('backend-engineer')

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('task.list', {
        spaceId: 'space-1',
        limit: 200,
      })
      expect(mockRpcCall).toHaveBeenCalledWith('space.member.list', {
        spaceId: 'space-1',
      })
    })
  })

  it('shows the four status column headers (no Member column)', async () => {
    renderBoard()

    await screen.findByText('Waiting')
    await screen.findByText('Active')
    await screen.findByText('Done')
    await screen.findByText('Failed')

    // Status replaces members as the layout axis — there is no Member header
    expect(screen.queryByText('Member')).not.toBeInTheDocument()
  })

  it('places a pending task in the Waiting column', async () => {
    renderBoard()

    await screen.findByText('Write repeatable disconnect checklist')
    const column = screen.getByTestId('board-col-waiting')
    expect(column).toContainElement(screen.getByTestId('board-card-task-1'))
  })

  it('filters tasks by the search query', async () => {
    renderBoard()

    await screen.findByText('Write repeatable disconnect checklist')

    fireEvent.change(screen.getByLabelText('Search tasks'), {
      target: { value: 'zzz-no-such-task' },
    })

    await waitFor(() => {
      expect(
        screen.queryByText('Write repeatable disconnect checklist'),
      ).not.toBeInTheDocument()
    })
    expect(screen.getByText('No tasks match your filters.')).toBeInTheDocument()
  })

  it('opens the initial task from a board deep link', async () => {
    const onOpenTask = vi.fn()

    renderBoard({ initialTaskId: 'task-1', onOpenTask })

    await screen.findByText('Write repeatable disconnect checklist')
    await waitFor(() => {
      expect(onOpenTask).toHaveBeenCalledWith(task, 'pending')
    })
  })

  it('exposes a New task button and a per-card actions menu', async () => {
    renderBoard()

    await screen.findByText('Write repeatable disconnect checklist')
    expect(screen.getByTestId('board-new-task')).toBeInTheDocument()
    expect(screen.getByTestId('board-card-menu-task-1')).toBeInTheDocument()
  })

  it('opens the create dialog from the New task button', async () => {
    renderBoard()

    await screen.findByText('Write repeatable disconnect checklist')
    fireEvent.click(screen.getByTestId('board-new-task'))

    // Dialog opens via controlled state, so its fields render without needing
    // a Radix pointer interaction.
    expect(await screen.findByRole('button', { name: 'Create task' })).toBeInTheDocument()
    expect(screen.getByLabelText('Assignee')).toBeInTheDocument()
  })

  it('offers a New task button when the space has no tasks', async () => {
    mockRpcCall.mockImplementation((method: string) => {
      if (method === 'task.list') return Promise.resolve({ tasks: [] })
      if (method === 'space.member.list') return Promise.resolve({ members: [member] })
      return Promise.reject(new Error(`unexpected method ${method}`))
    })

    renderBoard()

    await screen.findByText('No tasks in this space yet.')
    expect(screen.getByTestId('board-new-task')).toBeInTheDocument()
  })
})
