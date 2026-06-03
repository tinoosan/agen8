import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({ projectId: 'proj-1', focusedProjectRoot: '/repo' }),
}))

const mockUseMissions = vi.fn()
vi.mock('../hooks/useMissions', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../hooks/useMissions')>()
  return { ...mod, useMissions: (...args: unknown[]) => mockUseMissions(...args) }
})

// MissionEditor and CreateMissionDialog are complex — stub them out
vi.mock('../components/mission/MissionEditor', () => ({
  default: ({ mission }: { mission: { title: string } }) => (
    <div data-testid="mission-editor">{mission.title}</div>
  ),
}))
vi.mock('../components/mission/CreateMissionDialog', () => ({
  default: ({ open }: { open: boolean }) => open ? <div data-testid="create-dialog" /> : null,
}))

const { default: Missions } = await import('./Missions')

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Missions />
    </QueryClientProvider>,
  )
}

const sampleMissions = [
  { id: 'mis-1', title: 'Ship v2', description: '', status: 'active', projectId: 'proj-1', spaceId: '', keyResults: [], createdAt: '', updatedAt: '' },
  { id: 'mis-2', title: 'Reduce latency', description: '', status: 'draft', projectId: 'proj-1', spaceId: '', keyResults: [], createdAt: '', updatedAt: '' },
]

describe('Missions page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseMissions.mockReturnValue({ data: sampleMissions, isLoading: false, isError: false, error: null })
  })

  it('renders mission list', () => {
    renderPage()
    expect(screen.getByText('Ship v2')).toBeInTheDocument()
    expect(screen.getByText('Reduce latency')).toBeInTheDocument()
  })

  it('renders status filter tabs', () => {
    renderPage()
    expect(screen.getAllByRole('button', { name: /all/i }).length).toBeGreaterThanOrEqual(1)
    expect(screen.getByRole('button', { name: /active/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /draft/i })).toBeInTheDocument()
  })

  it('filters missions by status', async () => {
    const user = userEvent.setup()
    renderPage()

    // Click the Active filter button (may include count in accessible name)
    const activeBtn = screen.getAllByRole('button').find(b => b.textContent?.match(/^Active/))
    expect(activeBtn).toBeTruthy()
    await user.click(activeBtn!)

    await waitFor(() => {
      expect(screen.getByText('Ship v2')).toBeInTheDocument()
      expect(screen.queryByText('Reduce latency')).not.toBeInTheDocument()
    })
  })

  it('shows loading skeleton while fetching', () => {
    mockUseMissions.mockReturnValue({ data: undefined, isLoading: true, isError: false, error: null })
    renderPage()
    // Skeleton renders multiple placeholder elements
    expect(document.querySelectorAll('.skeleton').length).toBeGreaterThan(0)
  })

  it('shows error state when fetch fails', () => {
    mockUseMissions.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('Network error'),
    })
    renderPage()
    expect(screen.getByText(/failed to load missions/i)).toBeInTheDocument()
  })

  it('renders add mission button', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /new mission/i })).toBeInTheDocument()
  })
})
