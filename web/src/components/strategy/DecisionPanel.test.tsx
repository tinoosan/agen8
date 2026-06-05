import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../lib/rpc', () => ({
  rpcCall: vi.fn(),
}))

import { rpcCall } from '../../lib/rpc'
import { DecisionPanel } from './DecisionPanel'

const mockRpcCall = vi.mocked(rpcCall)

function renderWithQueryClient(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('DecisionPanel', () => {
  beforeEach(() => {
    mockRpcCall.mockImplementation(async (method: string, params?: Record<string, unknown>) => {
      if (method === 'mission.list') {
        return { missions: [{ id: 'mission-1', title: 'Stabilize backend', status: 'completed' }] }
      }
      if (method === 'mission.kr.list') {
        return { keyResults: [] }
      }
      if (method === 'mission.kr.get') {
        return {
          keyResult: {
            id: params?.keyResultId,
            missionId: 'mission-1',
            title: 'Retained MCP backend tools operate through registered project/member context',
            measurementType: 'percentage',
            direction: 'increase',
            targetValue: 100,
            currentValue: 100,
            progressPercent: 100,
            lastMilestoneNotified: 100,
            status: 'completed',
            createdAt: '2026-06-05T12:00:00Z',
            updatedAt: '2026-06-05T12:00:00Z',
          },
        }
      }
      if (method === 'task.list') {
        return { tasks: [] }
      }
      return {}
    })
  })

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

  it('resolves related key result titles instead of showing raw ids', async () => {
    renderWithQueryClient(
      <DecisionPanel
        onClose={vi.fn()}
        projectId="proj-1"
        data={{
          decision: {
            id: 'dec-4',
            projectId: 'proj-1',
            source: 'agent',
            kind: 'log',
            title: 'Remove binary viewers',
            rationale: 'The document viewer is not retained for this phase.',
            keyResultRef: 'kr-7bdeba84-9bfd-4108-b594-720aad411f25',
            confidence: 0.94,
            createdAt: '2026-06-05T12:00:00Z',
          },
        }}
      />,
    )

    await waitFor(() => {
      expect(screen.getByText('Retained MCP backend tools operate through registered project/member context')).toBeInTheDocument()
    })
    expect(screen.queryByText('kr-7bdeba84-')).not.toBeInTheDocument()
  })
})
