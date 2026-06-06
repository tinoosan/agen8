import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { createTestQueryClient } from '../test/test-utils'

const mockUseDecision = vi.fn()
const mockUseDeleteDecision = vi.fn()
const mockDeleteMutateAsync = vi.fn()
const mockUseMissions = vi.fn()
const mockUseProjectKRs = vi.fn()
const mockUseProjectTasks = vi.fn()
const mockToastSuccess = vi.fn()
const mockToastError = vi.fn()

vi.mock('../hooks/useDecisions', () => ({
  useDecision: (...args: unknown[]) => mockUseDecision(...args),
  useDeleteDecision: () => mockUseDeleteDecision(),
}))

vi.mock('../hooks/useMissions', () => ({
  useMissions: (...args: unknown[]) => mockUseMissions(...args),
  useProjectKRs: (...args: unknown[]) => mockUseProjectKRs(...args),
}))

vi.mock('../hooks/useProjectTasks', () => ({
  useProjectTasks: (...args: unknown[]) => mockUseProjectTasks(...args),
}))

vi.mock('sonner', () => ({
  toast: {
    success: (...args: unknown[]) => mockToastSuccess(...args),
    error: (...args: unknown[]) => mockToastError(...args),
  },
}))

const { default: DecisionDetail } = await import('./DecisionDetail')

const DECISION = {
  id: 'dec-1',
  projectId: 'proj-1',
  source: 'agent',
  sourceIdentity: 'cfo',
  memberName: 'Codex backend engineer',
  kind: 'log',
  title: 'Adopt usage-based pricing',
  context: 'Pricing experiment for the next iteration.',
  rationale: 'Usage overage validates metering before a broader rollout.',
  alternativesRejected: 'Flat subscription does not test willingness to pay.',
  outcome: 'Shipped metering to 10% of accounts.',
  confidence: 0.8,
  createdAt: '2026-03-31T10:00:00Z',
  missionRef: 'mis-1',
  taskRef: 'task-1',
  tags: ['pricing', 'metering'],
}

function renderDetail(path = '/project/proj-1/decisions/dec-1') {
  const queryClient = createTestQueryClient()
  const { hook } = memoryLocation({ path })
  return render(
    <QueryClientProvider client={queryClient}>
      <Router hook={hook}>
        <DecisionDetail />
      </Router>
    </QueryClientProvider>,
  )
}

describe('DecisionDetail page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseDecision.mockReturnValue({ data: DECISION, isLoading: false, isError: false, error: null })
    mockUseDeleteDecision.mockReturnValue({
      mutateAsync: mockDeleteMutateAsync.mockResolvedValue(true),
      isPending: false,
    })
    mockUseMissions.mockReturnValue({ data: [{ id: 'mis-1', title: 'Stabilize backend' }] })
    mockUseProjectKRs.mockReturnValue({ data: new Map() })
    mockUseProjectTasks.mockReturnValue({ data: [{ id: 'task-1', title: 'Wire metering' }] })
  })

  it('renders the decision with rationale, actor, and resolved related links', () => {
    renderDetail()

    expect(screen.getByRole('heading', { name: 'Adopt usage-based pricing' })).toBeInTheDocument()
    expect(screen.getByText('Usage overage validates metering before a broader rollout.')).toBeInTheDocument()
    expect(screen.getByText('Flat subscription does not test willingness to pay.')).toBeInTheDocument()
    expect(screen.getAllByText('Codex backend engineer').length).toBeGreaterThan(0)
    expect(screen.getByText(/80% confidence/)).toBeInTheDocument()

    // Mission/Task refs resolve to titled links that route to their detail pages.
    expect(screen.getByRole('link', { name: /stabilize backend/i })).toHaveAttribute(
      'href',
      '/project/proj-1/missions/mis-1',
    )
    expect(screen.getByRole('link', { name: /wire metering/i })).toHaveAttribute(
      'href',
      '/project/proj-1/tasks/task-1',
    )
  })

  it('deletes the decision after confirming in the dialog', async () => {
    const user = userEvent.setup()
    renderDetail()

    await user.click(screen.getByRole('button', { name: /^delete$/i }))
    await user.click(screen.getByRole('button', { name: /delete decision/i }))

    expect(mockDeleteMutateAsync).toHaveBeenCalledWith('dec-1')
    await waitFor(() => expect(mockToastSuccess).toHaveBeenCalledWith('Decision deleted'))
  })

  it('renders a skeleton while the decision is loading', () => {
    mockUseDecision.mockReturnValue({ data: undefined, isLoading: true, isError: false, error: null })
    renderDetail()

    expect(screen.queryByRole('heading', { name: 'Adopt usage-based pricing' })).not.toBeInTheDocument()
  })

  it('shows an error message when the decision fails to load', () => {
    mockUseDecision.mockReturnValue({
      data: undefined,
      isLoading: false,
      isError: true,
      error: new Error('boom'),
    })
    renderDetail()

    expect(screen.getByText(/failed to load decision: boom/i)).toBeInTheDocument()
  })
})
