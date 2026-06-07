import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, within } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Task, DecisionView, MissionView } from '../../lib/types'

/* The feed pulls from three query hooks; mock them so we can render the real
 * component against controlled data without a backend or auth. */
const mockTasks = vi.fn()
const mockDecisions = vi.fn()
const mockMissions = vi.fn()

vi.mock('../../hooks/useProjectTasks', () => ({
  useProjectTasks: () => mockTasks(),
}))
vi.mock('../../hooks/useDecisions', () => ({
  useRecentDecisions: () => mockDecisions(),
}))
vi.mock('../../hooks/useMissions', () => ({
  useMissions: () => mockMissions(),
}))

const { default: ActivityFeed } = await import('./ActivityFeed')

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()

function task(over: Partial<Task>): Task {
  return { id: 't', description: 'desc', status: 'pending', ...over }
}
function decision(over: Partial<DecisionView>): DecisionView {
  return {
    id: 'd',
    projectId: 'p',
    source: 'agent' as DecisionView['source'],
    title: 'A decision',
    rationale: 'r',
    confidence: 0.8,
    createdAt: minutesAgo(10),
    ...over,
  }
}
function mission(over: Partial<MissionView>): MissionView {
  return {
    id: 'm',
    projectId: 'p',
    title: 'A mission',
    status: 'active' as MissionView['status'],
    createdAt: minutesAgo(10),
    updatedAt: minutesAgo(10),
    ...over,
  }
}

const loading = { data: undefined, isLoading: true, isError: false }
const errored = { data: undefined, isLoading: false, isError: true }
const ok = (data: unknown) => ({ data, isLoading: false, isError: false })

function renderFeed() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <ActivityFeed projectId="p1" />
    </QueryClientProvider>,
  )
}

describe('ActivityFeed', () => {
  beforeEach(() => {
    mockTasks.mockReturnValue(ok([]))
    mockDecisions.mockReturnValue(ok([]))
    mockMissions.mockReturnValue(ok([]))
  })

  it('renders the empty state when there is no activity', () => {
    renderFeed()
    expect(screen.getByText(/No activity yet/i)).toBeTruthy()
  })

  it('shows the error state when any source fails', () => {
    mockTasks.mockReturnValue(errored)
    renderFeed()
    expect(screen.getByText(/Failed to load activity/i)).toBeTruthy()
  })

  it('shows a skeleton while any source is loading', () => {
    mockMissions.mockReturnValue(loading)
    const { container } = renderFeed()
    // Skeleton renders the project's `.skeleton` shimmer class.
    expect(container.querySelector('.skeleton')).toBeTruthy()
    expect(screen.queryByText(/No activity yet/i)).toBeNull()
  })

  it('renders milestone rows with actor, verb and subject', () => {
    mockTasks.mockReturnValue(
      ok([
        task({
          id: 'tA',
          title: 'Build the activity feed',
          status: 'succeeded',
          createdByLabel: 'Sol',
          claimedByMemberLabel: 'Nova',
          createdAt: minutesAgo(40),
          startedAt: minutesAgo(20),
          completedAt: minutesAgo(2),
        }),
      ]),
    )
    renderFeed()
    // Three milestones (created/started/completed) all reference the same subject.
    expect(screen.getAllByText('Build the activity feed').length).toBe(3)
    expect(screen.getByText(/completed task/i)).toBeTruthy()
    expect(screen.getByText(/started working on/i)).toBeTruthy()
    expect(screen.getByText(/created task/i)).toBeTruthy()
  })

  it('groups events under time-bucket dividers', () => {
    mockTasks.mockReturnValue(ok([task({ id: 'recent', createdAt: minutesAgo(1) })]))
    mockMissions.mockReturnValue(ok([mission({ id: 'mOld', createdAt: minutesAgo(120) })]))
    renderFeed()
    expect(screen.getByText('Just now')).toBeTruthy()
    expect(screen.getByText('Today')).toBeTruthy()
  })

  it('filters the stream by kind when a chip is clicked', () => {
    mockTasks.mockReturnValue(ok([task({ id: 'tF', title: 'A task subject', createdAt: minutesAgo(3) })]))
    mockDecisions.mockReturnValue(ok([decision({ id: 'dF', title: 'A decision subject', createdAt: minutesAgo(4) })]))
    renderFeed()

    // Both visible under "All".
    expect(screen.getByText('A task subject')).toBeTruthy()
    expect(screen.getByText('A decision subject')).toBeTruthy()

    // Click the "Decisions" filter tab → only the decision remains.
    const tablist = screen.getByRole('tablist')
    fireEvent.click(within(tablist).getByText('Decisions'))
    expect(screen.queryByText('A task subject')).toBeNull()
    expect(screen.getByText('A decision subject')).toBeTruthy()
  })
})
