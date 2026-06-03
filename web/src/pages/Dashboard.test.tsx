import { describe, it, expect, beforeEach, vi } from 'vitest'
import { fireEvent, render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockUseProjectSpaces = vi.fn()
const mockUseSpaceList = vi.fn()
const mockUseSpaceRoster = vi.fn()
const mockUseMissions = vi.fn()
const mockDashboardContextPanel = vi.fn()
const mockDecisionFeed = vi.fn()
const mockMissionSummary = vi.fn()
const mockRpcCall = vi.fn()
const notificationHandlers = new Map<string, Array<(notif: { params?: unknown }) => void>>()

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({
    projectId: 'proj-1',
    focusedProjectRoot: '/repo',
    focusedSpaceId: null,
    activeView: 'dashboard',
    setFocusedSpaceId: vi.fn(),
  }),
}))

vi.mock('../hooks/useProjectSpaces', () => ({
  useProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
}))

vi.mock('../hooks/useSpaceStatus', () => ({
  useSpaceManifest: () => ({ data: null }),
  useSpaceRoster: (...args: unknown[]) => mockUseSpaceRoster(...args),
  useSpaceStatus: () => ({ data: null }),
}))

vi.mock('../hooks/useMetrics', () => ({
  useProjectMetrics: () => ({ data: null }),
}))

vi.mock('../hooks/useMissions', () => ({
  useMissions: (...args: unknown[]) => mockUseMissions(...args),
}))

vi.mock('../hooks/useCountUp', () => ({
  useCountUp: (v: number) => v,
}))

vi.mock('../hooks/useEscalations', () => ({
  useEscalationSSE: () => {},
  usePendingEscalations: () => ({ data: [] }),
}))

vi.mock('../hooks/useOpActions', () => ({
  useOpActionSSE: () => {},
  usePendingOpActions: () => ({ data: [] }),
}))

vi.mock('../hooks/useOperatorMetrics', () => ({
  useOperatorMetrics: () => ({ data: null }),
}))

vi.mock('../hooks/useKeyboardShortcuts', () => ({
  useKeyboardShortcuts: () => {},
}))

vi.mock('../hooks/useOperatorNotifications', () => ({
  useOperatorNotifications: () => ({ data: [] }),
}))

vi.mock('../hooks/useSpace', () => ({
  useSpaceList: (...args: unknown[]) => mockUseSpaceList(...args),
}))

vi.mock('../lib/rpc', () => ({
  onNotification: (method: string, handler: (notif: { params?: unknown }) => void) => {
    const handlers = notificationHandlers.get(method) ?? []
    handlers.push(handler)
    notificationHandlers.set(method, handlers)
    return () => {
      notificationHandlers.set(method, (notificationHandlers.get(method) ?? []).filter(item => item !== handler))
    }
  },
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

vi.mock('../lib/reconcile', () => ({
  getProductReconcileBadge: () => null,
  getProductReconcileDetail: () => null,
  getProductReconcileReason: () => null,
}))

vi.mock('../lib/spaceDisplayName', () => ({
  spaceDisplayName: (_spaceID?: string, spaceName?: string, name?: string) => name ?? spaceName ?? 'Space',
}))

vi.mock('../components/dashboard/MissionSummary', () => ({
  default: (props: unknown) => {
    mockMissionSummary(props)
    return <div data-testid="mission-summary" />
  },
}))
vi.mock('../components/dashboard/EscalationQueue', () => ({
  default: () => <div data-testid="escalation-queue" />,
}))
vi.mock('../components/dashboard/OpActionQueue', () => ({
  default: () => <div data-testid="op-action-queue" />,
}))
vi.mock('../components/dashboard/DecisionFeed', () => ({
  default: (props: unknown) => {
    mockDecisionFeed(props)
    return <div data-testid="decision-feed" />
  },
}))
vi.mock('../components/dashboard/DashboardContextPanel', () => ({
  default: (props: unknown) => {
    mockDashboardContextPanel(props)
    return null
  },
}))
vi.mock('./Metrics', () => ({
  default: () => <div data-testid="metrics-page" />,
}))
vi.mock('../components/PulseDot', () => ({
  default: () => <span />,
}))
vi.mock('../components/SpaceControls', () => ({
  default: () => <div data-testid="space-controls" />,
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
    notificationHandlers.clear()
    mockUseProjectSpaces.mockReturnValue({ data: [], isLoading: false, isError: false })
    mockUseSpaceList.mockReturnValue({ data: [] })
    mockUseSpaceRoster.mockReturnValue({ data: { roles: [] } })
    mockUseMissions.mockReturnValue({ data: [], isLoading: false, isError: false })
    mockRpcCall.mockResolvedValue({ tasks: [] })
  })

  it('renders without crashing', () => {
    renderPage()
    // Dashboard renders even with no spaces; any stable element is fine.
    expect(document.body).toBeTruthy()
  })

  it('shows empty state when no spaces', () => {
    renderPage()
    // With no spaces the page should not throw and show some content.
    expect(document.querySelector('[class]')).toBeTruthy()
  })

  it('does not crash when notification preference storage is unavailable', () => {
    const getItemSpy = vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
      throw new Error('storage unavailable')
    })

    try {
      expect(() => renderPage()).not.toThrow()
      expect(document.querySelector('[class]')).toBeTruthy()
    } finally {
      getItemSpy.mockRestore()
    }
  })

  it('counts only open visible spaces in the dashboard pulse', () => {
    mockUseSpaceList.mockReturnValue({
      data: [
        { id: 'space-open-1', title: 'Open 1', status: 'open' },
        { id: 'space-open-2', title: 'Open 2', status: 'open' },
        { id: 'space-archived', title: 'Archived', status: 'archived' },
        { id: 'space-deleted', title: 'Deleted', status: 'deleted' },
      ],
      isLoading: false,
      isError: false,
    })

    renderPage()

    expect(mockUseSpaceList).toHaveBeenCalledWith(expect.objectContaining({ projectId: 'proj-1', status: 'open' }))
    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('2 spaces are moving.')
    expect(screen.queryByText('5 spaces ready')).toBeNull()
  })

  it('shows active mission work even when metrics has no active spaces', () => {
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-1', title: 'Ops', status: 'open' }] })
    mockUseMissions.mockReturnValue({
      data: [{ id: 'mission-1', title: 'Reliability hardening', status: 'active' }],
      isLoading: false,
      isError: false,
    })

    renderPage()

    expect(mockUseMissions).toHaveBeenCalledWith('proj-1', 'active')
    expect(screen.getByRole('heading', { level: 2 }).textContent).toBe('1 space is moving.')
  })

  it('opens the dashboard context rail on the default route', () => {
    renderPage()

    expect(mockDashboardContextPanel).toHaveBeenCalled()
    const lastCall = mockDashboardContextPanel.mock.calls.at(-1)?.[0] as { open: boolean; panel: string }
    expect(lastCall?.open).toBe(true)
    expect(lastCall?.panel).toBe('overview')
  })

  it('uses includeDeleted space records for decision context catalogs', () => {
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-1', title: 'Market Research' }] })
    mockUseProjectSpaces.mockImplementation((_projectRoot?: string, options?: { includeDeleted?: boolean }) => {
      if (options?.includeDeleted) {
        return {
          data: [
            { spaceId: 'space-active', spaceName: 'market-research', status: 'active' },
            { spaceId: 'space-deleted', spaceName: 'market-research', status: 'deleted' },
          ],
          isLoading: false,
          isError: false,
        }
      }
      return {
        data: [{ spaceId: 'space-active', spaceName: 'market-research', status: 'active' }],
        isLoading: false,
        isError: false,
      }
    })

    renderPage()

    expect(mockDecisionFeed).toHaveBeenCalled()
    const lastCall = mockDecisionFeed.mock.calls.at(-1)?.[0] as { spaces?: Array<{ spaceId: string }> }
    expect(lastCall?.spaces?.map(space => space.spaceId)).toContain('space-deleted')
  })

  it('shows active missions below the decision feed in the work column', () => {
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-1', title: 'Market Research' }] })
    renderPage()

    expect(mockMissionSummary).toHaveBeenCalled()
    const lastCall = mockMissionSummary.mock.calls.at(-1)?.[0] as { projectId?: string; mode?: string }
    expect(lastCall).toMatchObject({ projectId: 'proj-1', mode: 'active' })

    const decisionFeed = screen.getByTestId('decision-feed')
    const missionSummary = screen.getByTestId('mission-summary')
    expect(
      decisionFeed.compareDocumentPosition(missionSummary) & Node.DOCUMENT_POSITION_FOLLOWING,
    ).toBeTruthy()
  })

  it('keeps the human loop surface visible when nothing is waiting on a person', () => {
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-1', title: 'Market Research' }] })

    renderPage()

    expect(screen.getByText('Human Loop')).toBeInTheDocument()
    expect(screen.getByTestId('escalation-queue')).toBeInTheDocument()
    expect(screen.getByTestId('op-action-queue')).toBeInTheDocument()
    expect(screen.getByText('Nothing needs a person right now.')).toBeInTheDocument()
    expect(document.querySelector('.dashboard-overview-grid')).not.toHaveClass('dashboard-overview-grid-work-only')
  })

  it('marks the main dashboard scroller active while scrolling', () => {
    renderPage()

    const scroller = document.querySelector('.dashboard-main-scroll')
    expect(scroller).not.toBeNull()
    expect(scroller).not.toHaveClass('dashboard-scroll-active')

    fireEvent.scroll(scroller!)

    expect(scroller).toHaveClass('dashboard-scroll-active')
  })

})
