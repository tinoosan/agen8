import { describe, it, expect, beforeEach, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { QueryClientProvider } from '@tanstack/react-query'
import EscalationQueue from './EscalationQueue'
import { createTestQueryClient } from '../../test/test-utils'
import type { EscalationView } from '../../lib/types'

const toastMock = vi.hoisted(() => ({
  success: vi.fn(),
  error: vi.fn(),
}))
const mockUsePendingEscalations = vi.fn()
const mockUseResolveEscalation = vi.fn()

vi.mock('sonner', () => ({
  toast: toastMock,
}))

vi.mock('../../hooks/useEscalations', () => ({
  usePendingEscalations: (...args: unknown[]) => mockUsePendingEscalations(...args),
  useResolveEscalation: (...args: unknown[]) => mockUseResolveEscalation(...args),
}))

const escalation: EscalationView = {
  id: 'esc-1',
  projectId: 'proj-1',
  source: 'agent',
  sourceMemberLabel: 'cto',
  category: 'code',
  urgency: 'high',
  title: 'Review deployment risk',
  description: 'A production rollout needs operator guidance.',
  recommendation: 'Approve rollout',
  confidence: 0.82,
  status: 'pending',
  createdAt: '2026-03-31T10:00:00Z',
}

function renderQueue() {
  const queryClient = createTestQueryClient()
  return render(
    <QueryClientProvider client={queryClient}>
      <EscalationQueue projectId="proj-1" initialSelectedId="esc-1" />
    </QueryClientProvider>,
  )
}

describe('EscalationQueue', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    toastMock.success.mockClear()
    toastMock.error.mockClear()
    mockUsePendingEscalations.mockReturnValue({
      data: [escalation],
      isLoading: false,
      isError: false,
      error: null,
    })
    mockUseResolveEscalation.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
      mutateAsync: vi.fn(),
    })
  })

  it('shows all escalation resolutions as labeled buttons in the expanded card', async () => {
    renderQueue()

    expect(screen.getByRole('button', { name: 'Approve' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Reject' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Redirect' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Defer' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Delegate' })).toBeInTheDocument()
  })

  it('switches into bulk action mode when an escalation is selected', () => {
    renderQueue()

    fireEvent.click(screen.getByRole('checkbox'))

    expect(screen.getByText('1 selected')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Approve selected' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Clear' })).toBeInTheDocument()
  })

  it('reports the number of escalations that bulk approval actually resolves', async () => {
    const mutateAsync = vi.fn().mockResolvedValue(undefined)
    mockUseResolveEscalation.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
      mutateAsync,
    })

    renderQueue()

    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: 'Approve selected' }))

    await waitFor(() => {
      expect(mutateAsync).toHaveBeenCalledWith({
        escalationId: 'esc-1',
        resolution: 'approve',
        resolvedBy: 'operator',
      })
    })
    await waitFor(() => expect(toastMock.success).toHaveBeenCalledWith('Approved 1 escalation'))
    expect(toastMock.error).not.toHaveBeenCalled()
  })

  it('surfaces bulk approval failures instead of reporting zero approvals', async () => {
    const mutateAsync = vi.fn().mockRejectedValue(new Error('escalation is already resolved'))
    mockUseResolveEscalation.mockReturnValue({
      isPending: false,
      mutate: vi.fn(),
      mutateAsync,
    })

    renderQueue()

    fireEvent.click(screen.getByRole('checkbox'))
    fireEvent.click(screen.getByRole('button', { name: 'Approve selected' }))

    await waitFor(() => {
      expect(toastMock.error).toHaveBeenCalledWith(
        'Failed to approve 1 escalation: escalation is already resolved',
      )
    })
    expect(toastMock.success).not.toHaveBeenCalledWith('Approved 0 escalations')
    expect(screen.getByText('1 selected')).toBeInTheDocument()
  })
})
