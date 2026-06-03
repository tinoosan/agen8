import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { createRouterWrapper } from '../test/test-utils'

const mockRpcCall = vi.fn()
const mockUseKeyResults = vi.fn()
const mockSetSpace = vi.fn()
const mockUseProjectSpaces = vi.fn()

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

vi.mock('../hooks/useMissions', () => ({
  useKeyResults: (...args: unknown[]) => mockUseKeyResults(...args),
  useUpdateKeyResult: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useUpdateKRProgress: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useDeleteKeyResult: () => ({ mutateAsync: vi.fn(), isPending: false }),
  useSetSpace: () => ({ mutateAsync: mockSetSpace, isPending: false }),
}))

vi.mock('../hooks/useAssignableProjectSpaces', () => ({
  useAssignableProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
  mergeProjectSpaces: (...groups: Array<Array<{ spaceId: string }>>) => {
    const seen = new Set<string>()
    return groups.flat().filter((space) => {
      if (seen.has(space.spaceId)) return false
      seen.add(space.spaceId)
      return true
    })
  },
}))

vi.mock('../components/mission/ProgressHistory', () => ({
  default: () => null,
}))

import MissionDetail from './MissionDetail'

beforeEach(() => {
  vi.clearAllMocks()
  mockRpcCall.mockImplementation(async (method: string) => {
    if (method === 'mission.get') {
      return {
        mission: {
          id: 'mission-1',
          projectId: 'actual-project-1',
          title: 'Test Mission',
          description: '',
          status: 'draft',
          createdAt: '2026-05-26T09:00:00Z',
          updatedAt: '2026-05-26T09:00:00Z',
        },
      }
    }
    throw new Error(`unexpected rpc method ${method}`)
  })
  mockUseKeyResults.mockReturnValue({
    data: [{
      id: 'kr-1',
      missionId: 'mission-1',
      title: 'KR needs space',
      status: 'open',
      progressPercent: 0,
      currentValue: 0,
      targetValue: 1,
      measurementType: 'boolean',
      direction: 'increase',
    }],
    isLoading: false,
    isError: false,
  })
  mockUseProjectSpaces.mockImplementation((projectId: string | null) => {
    if (projectId === 'actual-project-1') {
      return {
        data: [{ projectId, spaceId: 'space-backend', spaceName: 'backend', status: 'open', spaceOpen: true }],
      }
    }
    return { data: [] }
  })
  mockSetSpace.mockResolvedValue({ keyResult: { id: 'kr-1', missionId: 'mission-1', spaceId: 'space-backend' } })
})

describe('MissionDetail key result ownership', () => {
  it('loads spaces from the mission project id and assigns an unowned key result', async () => {
    const { Wrapper } = createRouterWrapper('/project/route-project/missions/mission-1')

    render(
      <Wrapper>
        <MissionDetail />
      </Wrapper>,
    )

    await screen.findByText('Test Mission')
    fireEvent.click(screen.getByText('Expand all'))
    fireEvent.pointerDown(await screen.findByText('Assign space'))
    fireEvent.click(await screen.findByText('backend'))

    await waitFor(() => {
      expect(mockSetSpace).toHaveBeenCalledWith({
        keyResultId: 'kr-1',
        missionId: 'mission-1',
        spaceId: 'space-backend',
      })
    })
  })
})
