import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { useStore } from '../lib/store'

const mockUseNavigation = vi.fn()
const mockUseMissions = vi.fn()
const mockUseAuth = vi.fn()
const mockMissionSummary = vi.fn()
const mockDecisionFeed = vi.fn()
const mockUseStrategyGraph = vi.fn()

// One node so the in-place node-search modal has a selectable result. The title
// is read off data.mission.title (see StrategyMapSearch.getNodeTitle).
const searchNode = {
  id: 'n1',
  type: 'mission',
  position: { x: 0, y: 0 },
  data: { mission: { title: 'Launch rocket', status: 'active' } },
}

vi.mock('../lib/routing', () => ({
  useNavigation: () => mockUseNavigation(),
  strategyMapLink: (projectId: string, focusNodeId?: string) =>
    `/project/${encodeURIComponent(projectId)}/strategy${focusNodeId ? `?focus=${focusNodeId}` : ''}`,
  filteredTasksLink: (projectId: string, status: string) =>
    status && status !== 'all' ? `/project/${projectId}/tasks?status=${status}` : `/project/${projectId}/tasks`,
  missionsPageLink: (projectId: string) => `/project/${projectId}/missions`,
  decisionsLink: (projectId: string) => `/project/${projectId}/decisions`,
}))

vi.mock('../components/strategy/useStrategyGraph', () => ({
  useStrategyGraph: (...args: unknown[]) => mockUseStrategyGraph(...args),
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
vi.mock('../components/dashboard/SinceYouWereAway', () => ({
  default: () => null,
}))
vi.mock('../components/dashboard/RecentlyShipped', () => ({
  default: () => null,
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
    useStore.getState().setStrategySearchOpen(false)
    mockUseNavigation.mockReturnValue({ projectId: 'proj-1', focusedProjectRoot: '/repo' })
    mockUseMissions.mockReturnValue({ data: [], isLoading: false })
    mockUseAuth.mockReturnValue({ user: { name: 'Ada Lovelace', email: 'ada@example.com' } })
    mockUseStrategyGraph.mockReturnValue({ nodes: [searchNode], edges: [], isLoading: false })
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

  it('does not render the dashboard context panel (rail has been removed)', () => {
    renderPage()

    // The context rail and its toggle button were removed when Missions and
    // Decisions got their own dedicated pages. The toggle button is gone.
    expect(screen.queryByTestId('dashboard-context-panel-toggle')).not.toBeInTheDocument()
    expect(screen.queryByTestId('dashboard-context-panel')).not.toBeInTheDocument()
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

  it('header search opens the context-map node search in place without leaving the dashboard', async () => {
    const location = memoryLocation({ path: '/project/proj-1/dashboard', record: true })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <Router hook={location.hook}>
          <Dashboard />
        </Router>
      </QueryClientProvider>,
    )

    await userEvent.click(screen.getByLabelText('Search the context map'))

    // The modal opens in place: the flag flips and the search input mounts,
    // and the URL stays on the dashboard (no jump to the context map).
    expect(useStore.getState().strategySearchOpen).toBe(true)
    expect(screen.getByPlaceholderText(/Search nodes/i)).toBeInTheDocument()
    expect(location.history).toEqual(['/project/proj-1/dashboard'])
  })

  it('selecting a node from the in-place search deep-links to that node on the context map', async () => {
    const location = memoryLocation({ path: '/project/proj-1/dashboard', record: true })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <Router hook={location.hook}>
          <Dashboard />
        </Router>
      </QueryClientProvider>,
    )

    await userEvent.click(screen.getByLabelText('Search the context map'))
    await userEvent.click(screen.getByText('Launch rocket'))

    // Picking a result closes the modal and navigates to the map focused on
    // that node — the map is the only surface that renders a graph node.
    expect(useStore.getState().strategySearchOpen).toBe(false)
    expect(location.history.at(-1)).toBe('/project/proj-1/strategy?focus=n1')
  })

  it('prompts for a project when none is focused', () => {
    mockUseNavigation.mockReturnValue({ projectId: null, focusedProjectRoot: null })
    renderPage()
    expect(screen.getByText('Select a project')).toBeInTheDocument()
  })

  it('redirects legacy ?panel=missions deep link to the missions page', async () => {
    const location = memoryLocation({ path: '/project/proj-1/dashboard?panel=missions', record: true })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <Router hook={location.hook}>
          <Dashboard />
        </Router>
      </QueryClientProvider>,
    )

    expect(location.history.at(-1)).toBe('/project/proj-1/missions')
  })

  it('redirects legacy ?panel=decisions deep link to the decisions page', async () => {
    const location = memoryLocation({ path: '/project/proj-1/dashboard?panel=decisions', record: true })
    const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
    render(
      <QueryClientProvider client={client}>
        <Router hook={location.hook}>
          <Dashboard />
        </Router>
      </QueryClientProvider>,
    )

    expect(location.history.at(-1)).toBe('/project/proj-1/decisions')
  })
})
