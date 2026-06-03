import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import { describe, expect, it, vi } from 'vitest'
import { DecisionPanel } from './DecisionPanel'

function renderWithQueryClient(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('DecisionPanel', () => {
  it('renders ask_user context and answers', () => {
    renderWithQueryClient(
      <DecisionPanel
        onClose={vi.fn()}
        projectId="proj-1"
        data={{
          decision: {
            id: 'dec-1',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'cfo',
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
            confidence: 0,
            createdAt: '2026-04-18T14:02:00Z',
          },
        }}
      />,
    )

    expect(screen.getByText('Context')).toBeInTheDocument()
    expect(screen.getByText('Choosing between pricing-packaging options determines what we test next.')).toBeInTheDocument()
    expect(screen.getByText('Questions')).toBeInTheDocument()
    expect(screen.getByText('Which pricing packaging should we prioritize?')).toBeInTheDocument()
    expect(screen.getByText(/Usage overage/)).toBeInTheDocument()
    expect(screen.getByText(/Lean toward usage overage because it validates metering/)).toBeInTheDocument()
    expect(screen.getByText('Recommendation: Flat subscription')).toBeInTheDocument()
  })

  it('renders invalidation conditions and blocking question markers', () => {
    renderWithQueryClient(
      <DecisionPanel
        onClose={vi.fn()}
        projectId="proj-1"
        data={{
          decision: {
            id: 'dec-2',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'cfo',
            kind: 'ask_user',
            title: 'Execution gate',
            rationale: '',
            context: 'Dependent work is waiting.',
            questions: [
              {
                id: 'start',
                text: 'Can dependent workstreams start?',
                type: 'multiple_choice',
                options: ['Yes', 'No'],
                blocking: true,
              },
            ],
            answers: [{ questionId: 'start', selectedOption: 'Yes' }],
            confidence: 0,
            createdAt: '2026-04-18T14:02:00Z',
          },
        }}
      />,
    )

    expect(screen.getByText('Blocking')).toBeInTheDocument()

    renderWithQueryClient(
      <DecisionPanel
        onClose={vi.fn()}
        projectId="proj-1"
        data={{
          decision: {
            id: 'dec-3',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'cfo',
            kind: 'log',
            title: 'Prioritize metered pricing',
            rationale: 'This tests willingness to pay.',
            invalidationConditions: ['Conversion drops below baseline'],
            confidence: 0.8,
            createdAt: '2026-04-18T14:02:00Z',
          },
        }}
      />,
    )

    expect(screen.getByText('Invalidation conditions')).toBeInTheDocument()
    expect(screen.getByText('Conversion drops below baseline')).toBeInTheDocument()
  })
})
