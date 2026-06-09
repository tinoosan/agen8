import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationItem, NotificationsListResult } from '../../lib/types'

/* The card reads the notification list and marks rows read; mock the hook
 * module so we render the real card against controlled data. */
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

  it('shows only standing alerts and excludes one-off events', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'a1', trigger: 'task.stale_queued', title: 'Task stuck in the queue' }),
          notif({ id: 'a2', trigger: 'backlog.high', title: 'Backlog is high' }),
          notif({ id: 'a3', trigger: 'task.overrun', title: 'Task overrunning' }),
          notif({ id: 'e1', trigger: 'task.completed', title: 'A task completed' }),
          notif({ id: 'e2', trigger: 'task.in_review', title: 'Awaiting review' }),
        ],
        unreadCount: 5,
      }),
    )
    renderCard()
    expect(screen.getByText('Needs attention')).toBeInTheDocument()
    expect(screen.getByText('3 alerts')).toBeInTheDocument()
    expect(screen.getByText('Task stuck in the queue')).toBeInTheDocument()
    expect(screen.getByText('Backlog is high')).toBeInTheDocument()
    expect(screen.getByText('Task overrunning')).toBeInTheDocument()
    expect(screen.queryByText('A task completed')).not.toBeInTheDocument()
    expect(screen.queryByText('Awaiting review')).not.toBeInTheDocument()
  })

  it('orders critical alerts before warnings', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'w1', trigger: 'task.stale_queued', severity: 'warning', title: 'Warn alert' }),
          notif({ id: 'c1', trigger: 'backlog.high', severity: 'critical', title: 'Critical alert' }),
        ],
        unreadCount: 2,
      }),
    )
    renderCard()
    const titles = screen.getAllByText(/alert$/i).map((el) => el.textContent)
    expect(titles[0]).toBe('Critical alert')
    expect(titles[1]).toBe('Warn alert')
  })

  it('uses singular label for a single alert', () => {
    mockList.mockReturnValue(
      ok({ notifications: [notif({ id: 'a1' })], unreadCount: 1 }),
    )
    renderCard()
    expect(screen.getByText('1 alert')).toBeInTheDocument()
  })

  it('marks read and deep-links when a linked alert is clicked', () => {
    mockList.mockReturnValue(
      ok({
        notifications: [
          notif({ id: 'a9', title: 'Stuck task', link: { surface: 'task', url: '/project/p1/tasks/t9' } }),
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
