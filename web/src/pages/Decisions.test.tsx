import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('../lib/routing', () => ({
  decisionsPanelLink: (projectId: string) => `/project/${projectId}/dashboard?panel=decisions`,
  useNavigation: () => ({ projectId: 'proj-1', focusedProjectRoot: '/repo' }),
}))

const mockUseDecisionLog = vi.fn()
const mockUseDecisionStats = vi.fn()
const mockUseExportDecisions = vi.fn()
const mockUseDeleteDecision = vi.fn()
vi.mock('../hooks/useDecisions', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../hooks/useDecisions')>()
  return {
    ...mod,
    useDecisionLog: (...args: unknown[]) => mockUseDecisionLog(...args),
    useDecisionStats: (...args: unknown[]) => mockUseDecisionStats(...args),
    useExportDecisions: (...args: unknown[]) => mockUseExportDecisions(...args),
    useDeleteDecision: (...args: unknown[]) => mockUseDeleteDecision(...args),
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
  const mockDeleteMutateAsync = vi.fn()

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
            title: 'Use PostgreSQL',
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
    mockUseDeleteDecision.mockReturnValue({
      mutateAsync: mockDeleteMutateAsync.mockResolvedValue(true),
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

    expect(screen.getByText('Use PostgreSQL')).toBeInTheDocument()
    expect(screen.getByText('80%')).toBeInTheDocument()
  })

  it('renders logged decision details and linked refs in the expanded row', async () => {
    const user = userEvent.setup()
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

    await user.click(screen.getByRole('button', { name: /^toggle details for decision next iteration pricing packaging priority$/i }))

    expect(screen.getByText('Rationale')).toBeInTheDocument()
    expect(screen.getByText('Usage overage validates metering before a broader rollout.')).toBeInTheDocument()
    expect(screen.getByText('Alternatives')).toBeInTheDocument()
    expect(screen.getByText('Flat subscription does not test usage-based willingness to pay.')).toBeInTheDocument()
    expect(screen.getByText('Mission: mis-1234567890ab')).toBeInTheDocument()
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

  it('deletes a decision after confirmation', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.click(screen.getByRole('button', { name: /delete decision use postgresql/i }))
    await user.click(screen.getByRole('button', { name: /^delete decision$/i }))

    expect(mockDeleteMutateAsync).toHaveBeenCalledWith('dec-1')
  })

  it('renders metric cards from decision stats', () => {
    mockUseDecisionStats.mockReturnValue({
      data: { total: 12, lowConfidence: 3, unlinked: 5, withInvalidationConditions: 4 },
      isLoading: false,
      isError: false,
    })
    renderPage()

    expect(screen.getByText('Total')).toBeInTheDocument()
    expect(screen.getByText('Needs review')).toBeInTheDocument()
    expect(screen.getByText('Unlinked')).toBeInTheDocument()
    expect(screen.getByText('Revisit conditions')).toBeInTheDocument()
    expect(screen.getByText('12')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
    expect(screen.getByText('5')).toBeInTheDocument()
    expect(screen.getByText('4')).toBeInTheDocument()
  })

  it('hides the metric cards when there are no decisions', () => {
    mockUseDecisionStats.mockReturnValue({
      data: { total: 0, lowConfidence: 0, unlinked: 0, withInvalidationConditions: 0 },
      isLoading: false,
      isError: false,
    })
    renderPage()

    expect(screen.queryByText('Needs review')).not.toBeInTheDocument()
  })
})
