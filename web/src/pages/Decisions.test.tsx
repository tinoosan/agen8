import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('../lib/routing', () => ({
  decisionsPanelLink: (projectId: string) => `/project/${projectId}/dashboard?panel=decisions`,
  useNavigation: () => ({ projectId: 'proj-1', focusedProjectRoot: '/repo' }),
}))

const mockUseDecisionLog = vi.fn()
const mockUseExportDecisions = vi.fn()
const mockUseDeleteDecision = vi.fn()
vi.mock('../hooks/useDecisions', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../hooks/useDecisions')>()
  return {
    ...mod,
    useDecisionLog: (...args: unknown[]) => mockUseDecisionLog(...args),
    useExportDecisions: (...args: unknown[]) => mockUseExportDecisions(...args),
    useDeleteDecision: (...args: unknown[]) => mockUseDeleteDecision(...args),
  }
})

const mockUseProjectSpaces = vi.fn()
vi.mock('../hooks/useProjectSpaces', () => ({
  useProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
}))

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
            spaceId: 'space-eng',
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
    mockUseProjectSpaces.mockReturnValue({
      data: [{ spaceId: 'th-1', spaceName: 'Engineering' }],
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
    expect(screen.getByText('agent')).toBeInTheDocument()
  })

  it('renders ask_user context, answers, and linked refs in the expanded row', async () => {
    const user = userEvent.setup()
    mockUseDecisionLog.mockReturnValue({
      data: {
        decisions: [
          {
            id: 'dec-ask-1',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'cfo',
            spaceId: 'space-eng',
            kind: 'ask_user',
            title: 'Next iteration pricing packaging priority',
            rationale: '',
            context: 'Choosing between pricing-packaging options determines what we test next.',
            questions: [
              {
                id: 'pricing',
                text: 'Which pricing packaging should we prioritize?',
                type: 'multiple_choice',
                options: ['Flat subscription', 'Usage overage'],
                recommendation: 'Flat subscription',
              },
            ],
            answers: [
              {
                questionId: 'pricing',
                selectedOption: 'Usage overage',
                freeFormText: 'Lean toward usage overage because it validates metering.',
              },
            ],
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

    expect(screen.getByText('Context')).toBeInTheDocument()
    expect(screen.getByText('Choosing between pricing-packaging options determines what we test next.')).toBeInTheDocument()
    expect(screen.getByText('Questions')).toBeInTheDocument()
    expect(screen.getByText('Which pricing packaging should we prioritize?')).toBeInTheDocument()
    expect(screen.getByText('Recommendation: Flat subscription')).toBeInTheDocument()
    expect(screen.getByText(/Answer: Lean toward usage overage because it validates metering\./)).toBeInTheDocument()
    expect(screen.getByText('Mission: mis-1234567890ab')).toBeInTheDocument()
    expect(screen.queryByText('Plan: plan-1234567890ab')).not.toBeInTheDocument()
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
})
