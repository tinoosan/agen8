import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationItem, NotificationsListResult } from '../../lib/types'

/* The card reads the notification list and marks rows read; mock the hook
 * module so we render the real card against controlled data. The card groups
 * standing alerts by trigger type, names the type once per group, and leads each
 * task row with the task name carried in structured metadata — so these tests
 * assert group headers + metadata-driven rows, not per-row titles. */
const mockList = vi.fn()
const markRead = vi.fn()

vi.mock('../../hooks/useNotifications', () => ({
  useNotifications: () => mockList(),
  useMarkNotificationRead: () => ({ mutate: markRead }),
}))

const navigate = vi.fn()
vi.mock('wouter', () => ({
  useLocation: () => ['/', navigate],
}))

const { default: NeedsAttention } = await import('./NeedsAttention')

function notif(over: Partial<NotificationItem>): NotificationItem {
  return {
    id: 'n1',
    userId: 'u1',
    source: 'agen8',
    trigger: 'task.stale_queued',
    severity: 'warning',
    title: 'Task stuck in the queue',
    body: 'Sat unclaimed for a while.',
    createdAt: new Date(Date.now() - 5 * 60_000).toISOString(),
    ...over,
  }
}

const ok = (data: NotificationsListResult) => ({ data })

function renderCard(projectId: string | null = 'p1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <NeedsAttention projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('NeedsAttention', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList.mockReturnValue(ok({ notifications: [], unreadCount: 0 }))
  })

  it('renders nothing without a project', () => {
    const { container } = renderCard(null)
    expect(container).toBeEmptyDOMElement()
  })

  it('renders nothing when there are no standing alerts', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'e1', trigger: 'task.completed', title: 'A task completed' }),
          notif({ id: 'e2', trigger: 'task.in_review', title: 'Awaiting review' }),
        ],
        unreadCount: 2,
      }),
    )
    const { container } = renderCard()
    expect(container).toBeEmptyDOMElement()
  })

  it('groups standing alerts by type, names the type once, and leads rows with the task name', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'a1', trigger: 'task.stale_queued', metadata: { taskTitle: 'Build pipeline', duration: '2h' } }),
          notif({ id: 'a2', trigger: 'task.stale_queued', metadata: { taskTitle: 'Indexer job', duration: '1h' } }),
          notif({ id: 'b1', trigger: 'backlog.high', body: 'Backlog is high' }),
          notif({ id: 'e1', trigger: 'task.completed', title: 'A task completed' }),
          notif({ id: 'e2', trigger: 'task.in_review', title: 'Awaiting review' }),
        ],
        unreadCount: 5,
      }),
    )
    renderCard()

    expect(screen.getByText('Needs attention')).toBeInTheDocument()
    // Total counts every standing alert (2 stale + 1 backlog), not the groups.
    expect(screen.getByText('3 alerts')).toBeInTheDocument()

    // The type is stated exactly once as a group header, not repeated per row.
    expect(screen.getAllByText('Stuck in the queue')).toHaveLength(1)
    expect(screen.getByText('Backlog')).toBeInTheDocument()

    // Rows lead with the task name from structured metadata.
    expect(screen.getByText('Build pipeline')).toBeInTheDocument()
    expect(screen.getByText('Indexer job')).toBeInTheDocument()
    // ...and surface the duration alongside it.
    expect(screen.getByText('2h')).toBeInTheDocument()

    // One-off events never appear in the attention card.
    expect(screen.queryByText('A task completed')).not.toBeInTheDocument()
    expect(screen.queryByText('Awaiting review')).not.toBeInTheDocument()
  })

  it('shows a per-group count for groups with more than one alert', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'a1', trigger: 'task.stale_queued', metadata: { taskTitle: 'Task A' } }),
          notif({ id: 'a2', trigger: 'task.stale_queued', metadata: { taskTitle: 'Task B' } }),
          notif({ id: 'a3', trigger: 'task.stale_queued', metadata: { taskTitle: 'Task C' } }),
        ],
        unreadCount: 3,
      }),
    )
    renderCard()
    // The group header carries a count pill of "3" for the three-item group.
    expect(screen.getByText('Stuck in the queue')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('orders critical groups before warning groups', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'w1', trigger: 'task.stale_queued', severity: 'warning', metadata: { taskTitle: 'Warn task' } }),
          notif({ id: 'c1', trigger: 'backlog.high', severity: 'critical', body: 'Critical backlog' }),
        ],
        unreadCount: 2,
      }),
    )
    renderCard()
    // Group headers, in DOM order: the critical backlog group sorts first.
    const headers = screen.getAllByText(/^(Backlog|Stuck in the queue)$/).map((el) => el.textContent)
    expect(headers[0]).toBe('Backlog')
    expect(headers[1]).toBe('Stuck in the queue')
  })

  it('uses singular label for a single alert', () => {
    mockList.mockReturnValue(
      ok({ notifications: [notif({ id: 'a1', metadata: { taskTitle: 'Lone task' } })], unreadCount: 1 }),
    )
    renderCard()
    expect(screen.getByText('1 alert')).toBeInTheDocument()
  })

  it('caps rows per group and reveals the rest behind a "+N more" toggle', () => {
    const many = Array.from({ length: 7 }, (_, i) =>
      notif({
        id: `s${i}`,
        trigger: 'task.stale_queued',
        createdAt: new Date(Date.UTC(2026, 0, 1, 0, 0, i)).toISOString(),
        metadata: { taskTitle: `Task ${i}` },
      }),
    )
    mockList.mockReturnValue(ok({ notifications: many, unreadCount: 7 }))
    renderCard()

    expect(screen.getByText('7 alerts')).toBeInTheDocument()
    // Only the first four rows render; the rest are collapsed.
    expect(screen.getByText('Task 6')).toBeInTheDocument()
    expect(screen.getByText('Task 3')).toBeInTheDocument()
    expect(screen.queryByText('Task 2')).not.toBeInTheDocument()

    // The overflow affordance expands the group, then collapses it again.
    fireEvent.click(screen.getByText('+3 more'))
    expect(screen.getByText('Task 2')).toBeInTheDocument()
    expect(screen.getByText('Task 0')).toBeInTheDocument()
    fireEvent.click(screen.getByText('Show less'))
    expect(screen.queryByText('Task 2')).not.toBeInTheDocument()
  })

  it('marks read and deep-links when a linked alert is clicked', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({
            id: 'a9',
            metadata: { taskTitle: 'Stuck task' },
            link: { surface: 'task', url: '/project/p1/tasks/t9' },
          }),
        ],
        unreadCount: 1,
      }),
    )
    renderCard()
    fireEvent.click(screen.getByText('Stuck task'))
    expect(markRead).toHaveBeenCalledWith({ id: 'a9' })
    expect(navigate).toHaveBeenCalledWith('/project/p1/tasks/t9')
  })
})
