import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
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

  it('opens the dashboard context panel on the overview route', () => {
    renderPage()

    expect(mockDashboardContextPanel).toHaveBeenCalled()
    const lastCall = mockDashboardContextPanel.mock.calls.at(-1)?.[0] as { open: boolean; panel: string }
    expect(lastCall?.open).toBe(true)
    expect(lastCall?.panel).toBe('overview')
  })

  it('prompts for a project when none is focused', () => {
    mockUseNavigation.mockReturnValue({ projectId: null, focusedProjectRoot: null })
    renderPage()
    expect(screen.getByText('Select a project')).toBeInTheDocument()
  })
})
