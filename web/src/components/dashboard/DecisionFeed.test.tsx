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
    onNotification: vi.fn(() => () => {}),
  }
})

import { rpcCall } from '../../lib/rpc'
import DecisionFeed from './DecisionFeed'

const mockRpcCall = vi.mocked(rpcCall)

function renderWithQueryClient(ui: ReactElement) {
  const queryClient = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(<QueryClientProvider client={queryClient}>{ui}</QueryClientProvider>)
}

describe('DecisionFeed', () => {
  beforeEach(() => {
    mockRpcCall.mockImplementation(async (method: string) => {
      if (method === 'decision.list') {
        return {
          decisions: [{
            id: 'dec-1',
            projectId: 'proj-1',
            source: 'agent',
            sourceIdentity: 'member-95fed2e1ebce6ce6',
            memberId: 'member-95fed2e1ebce6ce6',
            memberName: 'Codex backend engineer',
            kind: 'log',
            title: 'Hard-cut stale message remnants',
            rationale: 'Removed stale client metadata from the retained MCP setup.',
            confidence: 0.9,
            createdAt: new Date().toISOString(),
          }],
        }
      }
      if (method === 'mission.list') return { missions: [] }
      if (method === 'mission.kr.list') return { keyResults: [] }
      if (method === 'task.list') return { tasks: [] }
      return {}
    })
  })

  it('shows the stamped member name instead of the raw member id', async () => {
    renderWithQueryClient(<DecisionFeed projectId="proj-1" />)

    await waitFor(() => {
      expect(screen.getByText('Codex backend engineer')).toBeInTheDocument()
    })
    expect(screen.queryByText('member-95fed2e1ebce6ce6')).not.toBeInTheDocument()
  })
})
