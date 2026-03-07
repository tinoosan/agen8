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
    id: `evt-${Math.random().toString(36).slice(2, 8)}`,
    kind: 'agent_message',
    title: 'Did something',
    status: 'ok',
    startedAt: new Date().toISOString(),
    ...overrides,
  }
}

function renderFeed(threadId: string | null = 'thread-1', teamId = 'team-1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })

  return render(
    <QueryClientProvider client={queryClient}>
      <ActivityFeed threadId={threadId} teamId={teamId} />
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
    expect(screen.getByText(/waiting for activity/i)).toBeInTheDocument()
  })

  it('renders event rows with role and humanized kind labels', () => {
    const events = [
      makeEvent({
        id: 'e1',
        kind: 'agent_message',
        title: 'Thinking about task',
        data: { role: 'coordinator' },
      }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    expect(screen.getByText('coordinator')).toBeInTheDocument()
    // agent_message is humanized to "Reply" by humanizeKind()
    expect(screen.getByText('Reply')).toBeInTheDocument()
    expect(screen.getByText('Thinking about task')).toBeInTheDocument()
  })

  it('renders multiple events', () => {
    const events = [
      makeEvent({ id: 'e1', title: 'Event one' }),
      makeEvent({ id: 'e2', title: 'Event two' }),
      makeEvent({ id: 'e3', title: 'Event three' }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    expect(screen.getByText('Event one')).toBeInTheDocument()
    expect(screen.getByText('Event two')).toBeInTheDocument()
    expect(screen.getByText('Event three')).toBeInTheDocument()
  })

  it('shows the full fetched activity window', () => {
    const events = Array.from({ length: 60 }, (_, i) =>
      makeEvent({ id: `e${i + 1}`, title: `Event ${i + 1}` }),
    )
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    expect(screen.getByText('Event 1')).toBeInTheDocument()
    expect(screen.getByText('Event 10')).toBeInTheDocument()
    expect(screen.getByText('Event 60')).toBeInTheDocument()
  })

  it('expands detail on click when event has detail', async () => {
    const user = userEvent.setup()
    const events = [
      makeEvent({
        id: 'e1',
        title: 'Has details',
        outputPreview: 'Detailed output content',
      }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    // Detail should not be visible initially
    expect(screen.queryByText('Detailed output content')).not.toBeInTheDocument()

    // Click to expand
    await user.click(screen.getByText('Has details'))

    // Detail should now be visible
    expect(screen.getByText('Detailed output content')).toBeInTheDocument()
  })

  it('collapses detail on second click', async () => {
    const user = userEvent.setup()
    const events = [
      makeEvent({
        id: 'e1',
        title: 'Toggle me',
        outputPreview: 'Detail content',
      }),
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
      makeEvent({
        id: 'e1',
        title: 'No details',
        textPreview: undefined,
        outputPreview: undefined,
        error: undefined,
      }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()

    await user.click(screen.getByText('No details'))
    // Nothing should change — no crash, no expansion
    expect(screen.getByText('No details')).toBeInTheDocument()
  })

  it('handles events without role or kind gracefully', () => {
    const events = [
      makeEvent({
        id: 'e1',
        kind: '',
        title: 'Minimal event',
        data: undefined,
      }),
    ]
    useActivity.mockReturnValue({ data: events, isLoading: false })
    renderFeed()
    expect(screen.getByText('Minimal event')).toBeInTheDocument()
  })

  it('calls useActivity with the correct options', () => {
    useActivity.mockReturnValue({ data: [], isLoading: false })
    renderFeed('thread-42', 'my-team-id')
    expect(useActivity).toHaveBeenCalledWith({
      threadId: 'thread-42',
      teamId: 'my-team-id',
      includeChildRuns: true,
      limit: 200,
    })
  })
})
