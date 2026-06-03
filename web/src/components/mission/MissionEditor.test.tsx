import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { createRouterWrapper } from '../../test/test-utils'

/* ── Mocks ─────────────────────────────────────────────── */

const mockCreateKR = vi.fn()
const mockSetSpace = vi.fn()
const mockUseKeyResults = vi.fn()
const mockUpdateMission = vi.fn()
const mockDeleteMission = vi.fn()
const mockUpdateKR = vi.fn()
const mockDeleteKR = vi.fn()
const mockUpdateKRProgress = vi.fn()
const mockUseProjectSpaces = vi.fn()

vi.mock('../../hooks/useMissions', () => ({
  useKeyResults: (...args: unknown[]) => mockUseKeyResults(...args),
  useCreateKeyResult: () => ({
    mutateAsync: mockCreateKR,
    isPending: false,
  }),
  useUpdateMission: () => ({
    mutateAsync: mockUpdateMission,
    isPending: false,
  }),
  useDeleteMission: () => ({
    mutateAsync: mockDeleteMission,
    isPending: false,
  }),
  useUpdateKeyResult: () => ({
    mutateAsync: mockUpdateKR,
    isPending: false,
  }),
  useDeleteKeyResult: () => ({
    mutateAsync: mockDeleteKR,
    isPending: false,
  }),
  useUpdateKRProgress: () => ({
    mutateAsync: mockUpdateKRProgress,
    isPending: false,
  }),
  useSetSpace: () => ({
    mutateAsync: mockSetSpace,
    isPending: false,
  }),
}))

const mockSpacesData = [
  { spaceId: 'space-backend', spaceName: 'backend', projectId: 'proj-1' },
  { spaceId: 'space-frontend', spaceName: 'frontend', projectId: 'proj-1' },
]

vi.mock('../../hooks/useAssignableProjectSpaces', () => ({
  useAssignableProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
}))

vi.mock('../../lib/routing', () => ({
  useNavigation: () => ({ projectId: '/home/user/project', focusedProjectRoot: '/home/user/project' }),
  missionDetailLink: (projectId: string, missionId: string) => `/project/${projectId}/missions/${missionId}`,
}))

vi.mock('./ProgressHistory', () => ({
  default: () => null,
}))

/* ── Import after mocks ────────────────────────────────── */

import MissionEditor from './MissionEditor'
import type { MissionView } from '../../lib/types'

const baseMission: MissionView = {
  id: 'mis-1',
  projectId: 'proj-1',
  title: 'Ship v2',
  description: 'Launch version 2',
  status: 'active',
  progressPercent: 50,
  keyResultCount: 0,
  createdAt: '2026-03-28T10:00:00Z',
  updatedAt: '2026-03-28T10:00:00Z',
}

function renderEditor() {
  mockUseKeyResults.mockReturnValue({ data: [], isLoading: false, isError: false })
  const { Wrapper } = createRouterWrapper('/project/proj-1/missions')
  return render(
    <Wrapper>
      <MissionEditor mission={baseMission} defaultExpanded />
    </Wrapper>,
  )
}

beforeEach(() => {
  vi.clearAllMocks()
  mockUseProjectSpaces.mockReturnValue({ data: mockSpacesData, isLoading: false })
})

describe('AddKeyResultForm space selector', () => {
  it('renders space selector when spaces are available', async () => {
    renderEditor()

    // Click "Add Key Result" button to show the form
    const addButton = screen.getByRole('button', { name: /add key result/i })
    fireEvent.click(addButton)

    expect(screen.getByText(/assigned space/i)).toBeTruthy()
  })

  it('does not call setSpace when no space is selected', async () => {
    mockCreateKR.mockResolvedValue({ keyResult: { id: 'kr-new', missionId: 'mis-1' } })

    renderEditor()

    const addButton = screen.getByRole('button', { name: /add key result/i })
    fireEvent.click(addButton)

    const titleInput = screen.getByPlaceholderText(/reduce build time/i)
    fireEvent.change(titleInput, { target: { value: 'Improve uptime' } })

    const submitButtons = screen.getAllByRole('button')
    const submitButton = submitButtons.find(btn => btn.textContent === 'Add Key Result')
    expect(submitButton).toBeTruthy()
    fireEvent.click(submitButton!)

    await waitFor(() => {
      expect(mockCreateKR).toHaveBeenCalledTimes(1)
    })

    expect(mockSetSpace).not.toHaveBeenCalled()
  })
})

describe('KeyResultEditorRow space labels', () => {
  it('lets an unassigned key result be assigned to a space from the row', async () => {
    mockSetSpace.mockResolvedValue({ keyResult: { id: 'kr-1', missionId: 'mis-1', spaceId: 'space-backend' } })
    mockUseKeyResults.mockReturnValue({
      data: [
        {
          id: 'kr-1',
          missionId: 'mis-1',
          title: 'Activation scope',
          status: 'open',
          progressPercent: 0,
          currentValue: 0,
          targetValue: 1,
          measurementType: 'binary',
          direction: 'increase',
        },
      ],
      isLoading: false,
      isError: false,
    })
    const { Wrapper } = createRouterWrapper('/project/proj-1/missions')

    render(
      <Wrapper>
        <MissionEditor mission={baseMission} defaultExpanded />
      </Wrapper>,
    )

    fireEvent.pointerDown(screen.getByText('Assign space'))
    fireEvent.click(await screen.findByText('backend'))

    await waitFor(() => {
      expect(mockSetSpace).toHaveBeenCalledWith({
        keyResultId: 'kr-1',
        spaceId: 'space-backend',
        missionId: 'mis-1',
      })
    })
  })

  it('resolves soft-deleted KR space assignments to the space label', () => {
    mockUseProjectSpaces.mockReturnValue({
      data: [
        {
          spaceId: 'space-deleted',
          spaceName: 'tnxp-thesis',
          status: 'deleted',
        },
      ],
      isLoading: false,
    })
    mockUseKeyResults.mockReturnValue({
      data: [
        {
          id: 'kr-1',
          missionId: 'mis-1',
          title: 'Risk assessment',
          status: 'open',
          progressPercent: 0,
          currentValue: 0,
          targetValue: 1,
          measurementType: 'binary',
          direction: 'increase',
          spaceId: 'space-deleted',
        },
      ],
      isLoading: false,
      isError: false,
    })
    const { Wrapper } = createRouterWrapper('/project/proj-1/missions')

    render(
      <Wrapper>
        <MissionEditor mission={baseMission} defaultExpanded />
      </Wrapper>,
    )

    expect(mockUseProjectSpaces).toHaveBeenCalledWith('/home/user/project', { includeDeleted: true })
    expect(screen.getByText('tnxp-thesis')).toBeInTheDocument()
    expect(screen.queryByText('space-deleted')).not.toBeInTheDocument()
  })
})
