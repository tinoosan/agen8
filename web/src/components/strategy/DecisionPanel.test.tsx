import { render, screen, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { ReactElement } from 'react'
import { beforeEach, describe, expect, it, vi } from 'vitest'

vi.mock('../../lib/rpc', () => {
  const rpcCall = vi.fn()
  return {
    rpcCall,
    rpcUnwrap: async (method: string, params: unknown, field: string) => {
      const res = (await rpcCall(method, params)) as Record<string, unknown>
      return res[field]
    },
    rpcUnwrapList: async (method: string, params: unknown, field: string) => {
      const res = (await rpcCall(method, params)) as Record<string, unknown[]>
      return res[field] ?? []
    },
  }
})

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

  it('renders logged decision rationale and alternatives', () => {
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
            kind: 'log',
            title: 'Next iteration pricing packaging priority',
            rationale: 'Usage overage validates metering before a broader rollout.',
            alternativesRejected: 'Flat subscription does not test usage-based willingness to pay.',
            confidence: 0.74,
            createdAt: '2026-04-18T14:02:00Z',
          },
        }}
      />,
    )

    expect(screen.getByText('Rationale')).toBeInTheDocument()
    expect(screen.getByText('Usage overage validates metering before a broader rollout.')).toBeInTheDocument()
    expect(screen.getByText('Alternatives rejected')).toBeInTheDocument()
    expect(screen.getByText('Flat subscription does not test usage-based willingness to pay.')).toBeInTheDocument()
  })

  it('renders invalidation conditions', () => {
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

  it('renders stamped member names instead of raw member ids', () => {
    renderWithQueryClient(
      <DecisionPanel
        onClose={vi.fn()}
        projectId="proj-1"
        data={{
          decision: {
            id: 'dec-member-name',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'member-95fed2e1ebce6ce6',
            memberId: 'member-95fed2e1ebce6ce6',
            memberName: 'Codex backend engineer',
            kind: 'log',
            title: 'Retain immutable actor labels',
            rationale: 'Decision cards should preserve the member name seen at write time.',
            confidence: 0.85,
            createdAt: '2026-06-05T12:00:00Z',
          },
        }}
      />,
    )

    expect(screen.getByText(/Codex backend engineer/)).toBeInTheDocument()
    expect(screen.queryByText(/member-95fed2e1ebce6ce6/)).not.toBeInTheDocument()
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
