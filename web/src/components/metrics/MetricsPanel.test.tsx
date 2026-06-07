import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { Task, ProjectMember } from '../../lib/types'

/* The panel pulls from two query hooks; mock them so we can render the real
 * component against controlled data without a backend or auth. */
const mockTasks = vi.fn()
const mockMembers = vi.fn()

vi.mock('../../hooks/useProjectTasks', () => ({
  useProjectTasks: () => mockTasks(),
}))
vi.mock('../../hooks/useProjectMembers', () => ({
  useProjectMembers: () => mockMembers(),
}))

const { default: MetricsPanel } = await import('./MetricsPanel')

const minutesAgo = (m: number) => new Date(Date.now() - m * 60_000).toISOString()

function task(over: Partial<Task>): Task {
  return { id: 't', description: 'desc', status: 'pending', ...over }
}
function member(over: Partial<ProjectMember>): ProjectMember {
  return { id: 'm', projectId: 'p', memberType: 'agent', lifecycleState: 'active', ...over }
}

const loading = { data: undefined, isLoading: true, isError: false }
const errored = { data: undefined, isLoading: false, isError: true }
const ok = (data: unknown) => ({ data, isLoading: false, isError: false })

function renderPanel() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <MetricsPanel projectId="p1" />
    </QueryClientProvider>,
  )
}

describe('MetricsPanel', () => {
  beforeEach(() => {
    mockTasks.mockReturnValue(ok([]))
    mockMembers.mockReturnValue(ok([]))
  })

  it('shows a skeleton while any source is loading', () => {
    mockMembers.mockReturnValue(loading)
    const { container } = renderPanel()
    expect(container.querySelector('.skeleton')).toBeTruthy()
  })

  it('shows the error state when any source fails', () => {
    mockTasks.mockReturnValue(errored)
    renderPanel()
    expect(screen.getByText(/Failed to load metrics/i)).toBeTruthy()
  })

  it('renders throughput tiles with derived figures', () => {
    mockTasks.mockReturnValue(
      ok([
        task({ id: 'a', status: 'pending' }),
        task({ id: 'b', status: 'pending' }),
        task({
          id: 'c',
          status: 'succeeded',
          createdAt: minutesAgo(40),
          startedAt: minutesAgo(38),
          completedAt: minutesAgo(20),
        }),
      ]),
    )
    renderPanel()
    expect(screen.getByText('Backlog')).toBeTruthy()
    // Backlog tile shows 2 queued.
    expect(screen.getByText('2')).toBeTruthy()
    // Completed tile shows "of 3 total tasks".
    expect(screen.getByText(/of 3 total tasks/i)).toBeTruthy()
  })

  it('renders the model and harness leaderboards with attributed work', () => {
    mockMembers.mockReturnValue(
      ok([member({ id: 'm1', model: 'claude-opus-4.7', harnessKind: 'claude-code' })]),
    )
    mockTasks.mockReturnValue(
      ok([
        task({
          id: 'a',
          status: 'succeeded',
          claimedByMemberId: 'm1',
          startedAt: minutesAgo(30),
          completedAt: minutesAgo(20),
        }),
      ]),
    )
    renderPanel()
    expect(screen.getByText('Model leaderboard')).toBeTruthy()
    expect(screen.getByText('Harness leaderboard')).toBeTruthy()
    // The model and harness keys both appear (in their respective tables/cards).
    expect(screen.getAllByText('claude-opus-4.7').length).toBeGreaterThan(0)
    expect(screen.getAllByText('claude-code').length).toBeGreaterThan(0)
  })

  it('shows an empty leaderboard message when nothing is attributable', () => {
    // A succeeded task but no member to attribute it to.
    mockTasks.mockReturnValue(ok([task({ id: 'a', status: 'succeeded' })]))
    renderPanel()
    expect(screen.getByText(/attributed to a model yet/i)).toBeTruthy()
    expect(screen.getByText(/attributed to a harness yet/i)).toBeTruthy()
  })

  it('renders an em dash for averages when there is no real data', () => {
    mockTasks.mockReturnValue(ok([task({ id: 'a', status: 'pending', createdAt: minutesAgo(5) })]))
    renderPanel()
    // Avg pickup latency + avg work time both have nothing to average → "—".
    const dashes = screen.getAllByText('—')
    expect(dashes.length).toBeGreaterThanOrEqual(2)
  })
})
