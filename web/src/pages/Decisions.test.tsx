import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('../lib/routing', () => ({
  dashboardLink: (projectId: string) => `/project/${projectId}/dashboard`,
  decisionDetailLink: (projectId: string, decisionId: string) => `/project/${projectId}/decisions/${decisionId}`,
  useNavigation: () => ({ projectId: 'proj-1', focusedProjectRoot: '/repo' }),
}))

const mockUseDecisionLog = vi.fn()
const mockUseDecisionStats = vi.fn()
const mockUseExportDecisions = vi.fn()
vi.mock('../hooks/useDecisions', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../hooks/useDecisions')>()
  return {
    ...mod,
    useDecisionLog: (...args: unknown[]) => mockUseDecisionLog(...args),
    useDecisionStats: (...args: unknown[]) => mockUseDecisionStats(...args),
    useExportDecisions: (...args: unknown[]) => mockUseExportDecisions(...args),
  }
})

const { default: Decisions } = await import('./Decisions')

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Decisions />
    </QueryClientProvider>,
  )
}

describe('Decisions page', () => {
  const mockMutateAsync = vi.fn()

  beforeEach(() => {
    vi.clearAllMocks()
    mockUseDecisionLog.mockReturnValue({
      data: {
        decisions: [
          {
            id: 'dec-1',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'cto',
            title: 'Use SQLite',
            rationale: 'Operationally simpler',
            confidence: 0.8,
            createdAt: '2026-03-31T10:00:00Z',
            taskRef: 'task-1',
          },
        ],
        total: 1,
      },
      isLoading: false,
      isError: false,
    })
    mockUseExportDecisions.mockReturnValue({
      mutateAsync: mockMutateAsync.mockResolvedValue([]),
      isPending: false,
    })
    mockUseDecisionStats.mockReturnValue({
      data: { total: 1, lowConfidence: 0, unlinked: 0, withInvalidationConditions: 0 },
      isLoading: false,
      isError: false,
    })
  })

  it('renders full decision log controls instead of the dashboard feed', () => {
    renderPage()

    expect(screen.getByRole('heading', { name: 'Decision Log' })).toBeInTheDocument()
    expect(screen.getByPlaceholderText(/search decisions/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^from$/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/^to$/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /oldest first|newest first/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /csv/i })).toBeInTheDocument()
  })

  it('renders decision rows', () => {
    renderPage()

    expect(screen.getByText('Use SQLite')).toBeInTheDocument()
    expect(screen.getByText('80%')).toBeInTheDocument()
  })

  it('links decision rows to the routed detail page', () => {
    mockUseDecisionLog.mockReturnValue({
      data: {
        decisions: [
          {
            id: 'dec-log-1',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'cfo',
            kind: 'log',
            title: 'Next iteration pricing packaging priority',
            rationale: 'Usage overage validates metering before a broader rollout.',
            alternativesRejected: 'Flat subscription does not test usage-based willingness to pay.',
            confidence: 0.8,
            createdAt: '2026-03-31T10:00:00Z',
            missionRef: 'mis-1234567890ab',
          },
        ],
        total: 1,
      },
      isLoading: false,
      isError: false,
    })

    renderPage()

    expect(
      screen.getByRole('link', { name: /next iteration pricing packaging priority/i }),
    ).toHaveAttribute('href', '/project/proj-1/decisions/dec-log-1')
  })

  it('shows pagination when there are multiple pages', () => {
    mockUseDecisionLog.mockReturnValue({
      data: {
        decisions: Array.from({ length: 20 }, (_, i) => ({
          id: `dec-${i}`,
          projectId: 'proj-1',
          source: 'agent',
          sourceIdentity: 'cto',
          title: `Decision ${i}`,
          rationale: 'reason',
          confidence: 0.8,
          createdAt: '2026-03-31T10:00:00Z',
        })),
        total: 40,
      },
      isLoading: false,
      isError: false,
    })
    renderPage()

    expect(screen.getByText('Page 1 of 2')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /previous/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /next/i })).toBeInTheDocument()
  })

  it('exports the current filtered view as CSV', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /csv/i }))

    expect(mockMutateAsync).toHaveBeenCalledWith(expect.objectContaining({
      projectId: 'proj-1',
      sort: 'newest',
    }))
  })

  it('renders decision stat tiles from decision.stats', () => {
    renderPage()
    // The page now leads with server-computed tiles.
    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByText('Low confidence')).toBeInTheDocument()
    expect(screen.getByText('Unlinked')).toBeInTheDocument()
    // The earlier, deliberately-removed tiles stay gone.
    expect(screen.queryByText('Needs review')).not.toBeInTheDocument()
    expect(screen.queryByText('Revisit conditions')).not.toBeInTheDocument()
  })

})
