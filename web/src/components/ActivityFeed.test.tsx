import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import ActivityFeed from './ActivityFeed'
import type { ActivityEvent } from '../lib/types'

// Mock rpc
vi.mock('../lib/rpc', () => ({
  rpcCall: vi.fn(),
  onNotification: vi.fn(() => () => {}),
}))

// Mock useActivity
vi.mock('../hooks/useActivity', () => ({
  useActivity: vi.fn(),
}))

const { useActivity } = await import('../hooks/useActivity') as { useActivity: ReturnType<typeof vi.fn> }

function makeEvent(overrides: Partial<ActivityEvent> = {}): ActivityEvent {
  return {
    seq: 1,
    type: 'agent_message',
    role: 'coordinator',
    summary: 'Did something',
    createdAt: new Date().toISOString(),
    ...overrides,
  }
}

function renderFeed(teamId = 'team-1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <ActivityFeed teamId={teamId} />
    </QueryClientProvider>,
  )
}

describe('ActivityFeed', () => {
  beforeEach(() => {
    vi.clearAllMocks()
  })

  it('shows empty state when no events', () => {
    useActivity.mockReturnValue({ data: [], isLoading: false })
    renderFeed()
    expect(screen.getByText(/no activity yet/i)).toBeInTheDocument()
  })

  it('renders event rows with role and type labels', () => {
    const events = [
      makeEvent({ seq: 1, role: 'coordinator', type: 'agent_message', summary: 'Thinking about task' }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    expect(screen.getByText('coordinator')).toBeInTheDocument()
    expect(screen.getByText('agent_message')).toBeInTheDocument()
    expect(screen.getByText('Thinking about task')).toBeInTheDocument()
  })

  it('renders multiple events', () => {
    const events = [
      makeEvent({ seq: 1, summary: 'Event one' }),
      makeEvent({ seq: 2, summary: 'Event two' }),
      makeEvent({ seq: 3, summary: 'Event three' }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    expect(screen.getByText('Event one')).toBeInTheDocument()
    expect(screen.getByText('Event two')).toBeInTheDocument()
    expect(screen.getByText('Event three')).toBeInTheDocument()
  })

  it('only shows last 50 events', () => {
    const events = Array.from({ length: 60 }, (_, i) =>
      makeEvent({ seq: i + 1, summary: `Event ${i + 1}` }),
    )
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    // First 10 events should not be shown (60 - 50 = 10)
    expect(screen.queryByText('Event 1')).not.toBeInTheDocument()
    expect(screen.queryByText('Event 10')).not.toBeInTheDocument()
    // Last event should be shown
    expect(screen.getByText('Event 60')).toBeInTheDocument()
    expect(screen.getByText('Event 11')).toBeInTheDocument()
  })

  it('expands detail on click when event has detail', async () => {
    const user = userEvent.setup()
    const events = [
      makeEvent({
        seq: 1,
        summary: 'Has details',
        detail: '{"key": "value"}',
      }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    // Detail should not be visible initially
    expect(screen.queryByText('{"key": "value"}')).not.toBeInTheDocument()

    // Click to expand
    await user.click(screen.getByText('Has details'))

    // Detail should now be visible
    expect(screen.getByText('{"key": "value"}')).toBeInTheDocument()
  })

  it('collapses detail on second click', async () => {
    const user = userEvent.setup()
    const events = [
      makeEvent({ seq: 1, summary: 'Toggle me', detail: 'Detail content' }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    // Click to expand
    await user.click(screen.getByText('Toggle me'))
    expect(screen.getByText('Detail content')).toBeInTheDocument()

    // Click again to collapse
    await user.click(screen.getByText('Toggle me'))
    expect(screen.queryByText('Detail content')).not.toBeInTheDocument()
  })

  it('does not expand when event has no detail', async () => {
    const user = userEvent.setup()
    const events = [
      makeEvent({ seq: 1, summary: 'No details', detail: undefined }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    await user.click(screen.getByText('No details'))
    // Nothing should change — no crash, no expansion
    expect(screen.getByText('No details')).toBeInTheDocument()
  })

  it('handles events without role or type gracefully', () => {
    const events = [
      makeEvent({ seq: 1, role: undefined, type: undefined, summary: 'Minimal event' }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()
    expect(screen.getByText('Minimal event')).toBeInTheDocument()
  })

  it('calls useActivity with the correct teamId', () => {
    useActivity.mockReturnValue({ data: [], isLoading: false })
    renderFeed('my-team-id')
    expect(useActivity).toHaveBeenCalledWith('my-team-id')
  })
})
