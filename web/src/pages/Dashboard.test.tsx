import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockUseNavigation = vi.fn()
const mockUseMissions = vi.fn()
const mockUseAuth = vi.fn()
const mockMissionSummary = vi.fn()
const mockDecisionFeed = vi.fn()
const mockDashboardContextPanel = vi.fn()

vi.mock('../lib/routing', () => ({
  useNavigation: () => mockUseNavigation(),
}))

vi.mock('../hooks/useMissions', () => ({
  useMissions: (...args: unknown[]) => mockUseMissions(...args),
}))

vi.mock('../hooks/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

vi.mock('../hooks/useKeyboardShortcuts', () => ({
  useKeyboardShortcuts: () => {},
}))

vi.mock('../components/dashboard/DecisionFeed', () => ({
  default: (props: unknown) => {
    mockDecisionFeed(props)
    return <div data-testid="decision-feed" />
  },
}))
vi.mock('../components/dashboard/MissionSummary', () => ({
  default: (props: unknown) => {
    mockMissionSummary(props)
    return <div data-testid="mission-summary" />
  },
}))
vi.mock('../components/dashboard/DashboardContextPanel', () => ({
  default: (props: unknown) => {
    mockDashboardContextPanel(props)
    return <div data-testid="dashboard-context-panel" />
  },
}))

const { default: Dashboard } = await import('./Dashboard')

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Dashboard />
    </QueryClientProvider>,
  )
}

describe('Dashboard page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    try { localStorage.clear() } catch { /* noop */ }
    mockUseNavigation.mockReturnValue({ projectId: 'proj-1', focusedProjectRoot: '/repo' })
    mockUseMissions.mockReturnValue({ data: [], isLoading: false })
    mockUseAuth.mockReturnValue({ user: { name: 'Ada Lovelace', email: 'ada@example.com' } })
  })

  it('greets the user and renders the work-in-motion column', () => {
    renderPage()

    expect(screen.getByRole('heading', { level: 1 }).textContent).toContain('Ada')
    expect(screen.getByTestId('decision-feed')).toBeInTheDocument()
    expect(screen.getByTestId('mission-summary')).toBeInTheDocument()
  })

  it('renders active mission work via MissionSummary', () => {
    renderPage()

    expect(mockUseMissions).toHaveBeenCalledWith('proj-1', 'active')
    const lastCall = mockMissionSummary.mock.calls.at(-1)?.[0] as { projectId?: string; mode?: string }
    expect(lastCall).toMatchObject({ projectId: 'proj-1', mode: 'active' })
  })

  it('renders the dashboard context drawer closed by default below the overlay breakpoint', () => {
    renderPage()

    expect(mockDashboardContextPanel).toHaveBeenCalled()
    const lastCall = mockDashboardContextPanel.mock.calls.at(-1)?.[0] as { open: boolean; overlay: boolean; panel: string }
    expect(lastCall?.overlay).toBe(true)
    expect(lastCall?.open).toBe(false)
    expect(lastCall?.panel).toBe('missions')
  })

  it('refresh invalidates the dashboard query families that feed visible work', async () => {
    const invalidateSpy = vi.spyOn(QueryClient.prototype, 'invalidateQueries')
    renderPage()

    await userEvent.click(screen.getByRole('button', { name: /refresh/i }))

    const invalidatedKeys = invalidateSpy.mock.calls
      .map(([arg]) => (arg as { queryKey?: unknown[] } | undefined)?.queryKey?.[0])

    expect(invalidatedKeys).toEqual(expect.arrayContaining([
      'mission.list',
      'keyResult.list',
      'keyResult.listAll',
      'keyResult.progressHistory',
      'decision.list',
      'decision.log',
      'project.tasks.board',
      'task.get',
    ]))
  })

  it('prompts for a project when none is focused', () => {
    mockUseNavigation.mockReturnValue({ projectId: null, focusedProjectRoot: null })
    renderPage()
    expect(screen.getByText('Select a project')).toBeInTheDocument()
  })
})
