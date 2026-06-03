import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { render, screen, waitFor } from '@testing-library/react'
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
      return Promise.reject(new Error(`unexpected method ${method}`))
    })
  })

  it('renders tasks grouped by agent in swimlane layout', async () => {
    renderBoard()

    // Task title appears in the swimlane
    await screen.findByText('Write repeatable disconnect checklist')

    // Agent name appears as a swimlane row label
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

  it('shows column headers for status groups', async () => {
    renderBoard()

    await screen.findByText('Member')
    await screen.findByText('Waiting')
    await screen.findByText('Active')
    await screen.findByText('Done')
    await screen.findByText('Failed')
  })

  it('opens the initial task from a board deep link', async () => {
    const onOpenTask = vi.fn()

    renderBoard({ initialTaskId: 'task-1', onOpenTask })

    await screen.findByText('Write repeatable disconnect checklist')
    await waitFor(() => {
      expect(onOpenTask).toHaveBeenCalledWith(task, 'pending')
    })
  })
})
