import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { createRouterWrapper } from '../../test/test-utils'

const mockUpdateKRProgress = vi.fn()
const mockRpcCall = vi.fn()

vi.mock('../../hooks/useMissions', () => ({
  useMissions: () => ({
    data: [{
      id: 'mission-1',
      projectId: 'project-1',
      title: 'Launch reliable onboarding',
      status: 'active',
      createdAt: '2026-05-26T09:00:00Z',
      updatedAt: '2026-05-26T09:00:00Z',
    }],
    isLoading: false,
  }),
  useKeyResults: () => ({
    data: [{
      id: 'kr-1',
      missionId: 'mission-1',
      title: 'Reach 80% activation',
      measurementType: 'percentage',
      direction: 'increase',
      unit: '%',
      baseline: 0,
      targetValue: 80,
      currentValue: 20,
      progressPercent: 25,
      lastMilestoneNotified: 0,
      spaceId: 'space-1',
      status: 'on_track',
      createdAt: '2026-05-26T09:00:00Z',
      updatedAt: '2026-05-26T09:00:00Z',
    }],
    isLoading: false,
  }),
  useUpdateKRProgress: () => ({
    mutateAsync: mockUpdateKRProgress,
    isPending: false,
  }),
}))

vi.mock('../../hooks/useEscalations', () => ({
  usePendingEscalations: () => ({ data: [], isLoading: false }),
}))

vi.mock('../../hooks/useDecisions', () => ({
  useRecentDecisions: () => ({ data: [], isLoading: false }),
}))

vi.mock('../../hooks/useSpaceDetail', () => ({
  useSpaceDetail: () => ({
    inspectorEvents: [],
    query: { isLoading: false },
  }),
}))

vi.mock('../../hooks/useSpace', () => ({
  useSpaceMemberList: () => ({ data: [], isLoading: false }),
}))

vi.mock('../../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

import SpaceOverviewTab from './SpaceOverviewTab'

beforeEach(() => {
  vi.clearAllMocks()
  mockUpdateKRProgress.mockResolvedValue({ keyResult: { id: 'kr-1' } })
  mockRpcCall.mockImplementation(async (method: string) => {
    if (method === 'task.list') return { tasks: [] }
    throw new Error(`unexpected rpc method ${method}`)
  })
})

describe('SpaceOverviewTab progress reporting', () => {
  it('lets the user update progress for the space key result', async () => {
    const { Wrapper } = createRouterWrapper('/project/project-1/space/space-1')

    render(
      <Wrapper>
        <SpaceOverviewTab spaceId="space-1" projectId="project-1" />
      </Wrapper>,
    )

    fireEvent.click(screen.getByRole('button', { name: /update progress/i }))
    fireEvent.change(screen.getByLabelText('Progress value'), { target: { value: '48' } })
    fireEvent.change(screen.getByLabelText('Progress note'), { target: { value: 'Activation event shipped' } })
    fireEvent.click(screen.getByRole('button', { name: 'Save' }))

    await waitFor(() => {
      expect(mockUpdateKRProgress).toHaveBeenCalledWith({
        keyResultId: 'kr-1',
        missionId: 'mission-1',
        value: 48,
        note: 'Activation event shipped',
      })
    })
  })
})
