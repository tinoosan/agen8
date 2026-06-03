import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import DecisionFeed from './DecisionFeed'

const mockUseRecentDecisions = vi.fn()
const mockUseMissions = vi.fn()
const mockUseProjectKRs = vi.fn()
const mockUseProjectTasks = vi.fn()
const mockUsePendingOpActions = vi.fn()

vi.mock('../../hooks/useDecisions', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../../hooks/useDecisions')>()
  return {
    ...mod,
    useRecentDecisions: (...args: unknown[]) => mockUseRecentDecisions(...args),
  }
})

vi.mock('../../hooks/useMissions', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../../hooks/useMissions')>()
  return {
    ...mod,
    useMissions: (...args: unknown[]) => mockUseMissions(...args),
    useProjectKRs: (...args: unknown[]) => mockUseProjectKRs(...args),
  }
})

vi.mock('../../hooks/useProjectTasks', () => ({
  useProjectTasks: (...args: unknown[]) => mockUseProjectTasks(...args),
}))

vi.mock('../../hooks/useOpActions', () => ({
  usePendingOpActions: (...args: unknown[]) => mockUsePendingOpActions(...args),
}))

function renderFeed() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <DecisionFeed projectId="proj-1" />
    </QueryClientProvider>,
  )
}

describe('DecisionFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseMissions.mockReturnValue({ data: [], isLoading: false })
    mockUseProjectKRs.mockReturnValue({ data: new Map(), isLoading: false })
    mockUseProjectTasks.mockReturnValue({ data: [], isLoading: false })
    mockUsePendingOpActions.mockReturnValue({ data: [], isLoading: false })
  })

  it('renders ask_user context, answers, recommendation, and refs when expanded', async () => {
    const user = userEvent.setup()
    mockUseRecentDecisions.mockReturnValue({
      data: [
        {
          id: 'dec-ask-1',
          projectId: 'proj-1',
          source: 'agent',
          sourceIdentity: 'cfo',
          kind: 'ask_user',
          title: 'Asked 1 question',
          rationale: '',
          context: 'Choose the strongest pricing signal to optimize next.',
          questions: [
            {
              id: 'metric',
              text: 'Which signal should dominate the next experiment?',
              type: 'multiple_choice',
              options: ['pricingKnown', 'retention'],
              recommendation: 'pricingKnown',
            },
          ],
          answers: [
            {
              questionId: 'metric',
              selectedOption: 'pricingKnown',
            },
          ],
          confidence: 0.8,
          createdAt: '2026-04-18T14:02:00Z',
          missionRef: 'mis-1234567890ab',
          planRef: 'plan-1234567890ab',
        },
      ],
      isLoading: false,
      isError: false,
    })
    mockUseMissions.mockReturnValue({
      data: [{ id: 'mis-1234567890ab', projectId: 'proj-1', title: 'Quarterly Planning', status: 'active', createdAt: '', updatedAt: '' }],
      isLoading: false,
    })
    renderFeed()

    await user.click(screen.getByRole('button', { name: /Asked 1 question/i }))

    expect(screen.getByText('Context')).toBeInTheDocument()
    expect(screen.getByText('Choose the strongest pricing signal to optimize next.')).toBeInTheDocument()
    expect(screen.getByText('Questions')).toBeInTheDocument()
    expect(screen.getByText('Which signal should dominate the next experiment?')).toBeInTheDocument()
    expect(screen.getByText('Recommendation: pricingKnown')).toBeInTheDocument()
    expect(screen.getByText('Answer: pricingKnown')).toBeInTheDocument()
    expect(screen.getAllByText(/Mission: Quarterly Planning/)).toHaveLength(2)
    expect(screen.queryByText(/Plan: Market Rollout Plan/)).not.toBeInTheDocument()
    expect(screen.queryByText(/mis-1234567890ab/)).not.toBeInTheDocument()
    expect(screen.queryByText(/plan-1234567890ab/)).not.toBeInTheDocument()
  })

  it('renders the resolved member name for operator-authored decisions', () => {
    mockUseRecentDecisions.mockReturnValue({
      data: [
        {
          id: 'dec-operator-1',
          projectId: 'proj-1',
          source: 'operator',
          sourceIdentity: 'operator',
          memberId: 'operator',
          memberName: 'Santino Onyeme',
          kind: 'log',
          title: 'Resolved escalation',
          rationale: 'No action required.',
          confidence: 0.8,
          createdAt: '2026-04-18T14:02:00Z',
        },
      ],
      isLoading: false,
      isError: false,
    })

    renderFeed()

    expect(screen.getByText('Santino Onyeme')).toBeInTheDocument()
    expect(screen.queryByText('operator')).not.toBeInTheDocument()
  })
})
