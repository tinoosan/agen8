import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import type { NotificationItem, NotificationsListResult } from '../../lib/types'

/* The inbox reads + mutates through the notification hooks; mock the module so
 * we render the real popover UI against controlled data and observe the
 * mutations it fires. */
const mockList = vi.fn()
const markRead = vi.fn()
const markAllRead = vi.fn()
const dismiss = vi.fn()

vi.mock('../../hooks/useNotifications', () => ({
  useNotifications: () => mockList(),
  useMarkNotificationRead: () => ({ mutate: markRead }),
  useMarkAllNotificationsRead: () => ({ mutate: markAllRead }),
  useDismissNotification: () => ({ mutate: dismiss }),
}))

const navigate = vi.fn()
vi.mock('wouter', () => ({
  useLocation: () => ['/', navigate],
}))

const { default: NotificationInbox } = await import('./NotificationInbox')

function notif(over: Partial<NotificationItem>): NotificationItem {
  return {
    id: 'n1',
    userId: 'u1',
    source: 'agen8',
    trigger: 'task.completed',
    severity: 'info',
    title: 'A task completed',
    body: 'Ship it was approved.',
    createdAt: new Date(Date.now() - 5 * 60_000).toISOString(),
    ...over,
  }
}

const ok = (data: NotificationsListResult) => ({ data })

function renderInbox(projectId: string | null = 'p1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <NotificationInbox projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('NotificationInbox', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockList.mockReturnValue(ok({ notifications: [], unreadCount: 0 }))
  })

  it('renders nothing without a project', () => {
    const { container } = renderInbox(null)
    expect(container).toBeEmptyDOMElement()
  })

  it('shows the unread count badge on the bell', () => {
    mockList.mockReturnValue(ok({ notifications: [notif({})], unreadCount: 3 }))
    renderInbox()
    expect(screen.getByLabelText('Notifications (3 unread)')).toBeInTheDocument()
    expect(screen.getByText('3')).toBeInTheDocument()
  })

  it('caps the badge at 99+', () => {
    mockList.mockReturnValue(ok({ notifications: [], unreadCount: 150 }))
    renderInbox()
    expect(screen.getByText('99+')).toBeInTheDocument()
  })

  it('lists notifications and the empty state', () => {
    mockList.mockReturnValue(ok({ notifications: [], unreadCount: 0 }))
    renderInbox()
    fireEvent.click(screen.getByLabelText('Notifications'))
    expect(screen.getByText("You're all caught up")).toBeInTheDocument()
  })

  it('marks read and deep-links when a linked row is clicked', () => {
    const item = notif({
      id: 'n9',
      title: 'Review me',
      link: { surface: 'task', url: '/project/p1/tasks/t9' },
    })
    mockList.mockReturnValue(ok({ notifications: [item], unreadCount: 1 }))
    renderInbox()
    fireEvent.click(screen.getByLabelText('Notifications (1 unread)'))
    fireEvent.click(screen.getByText('Review me'))
    expect(markRead).toHaveBeenCalledWith({ id: 'n9' })
    expect(navigate).toHaveBeenCalledWith('/project/p1/tasks/t9')
  })

  it('dismisses a row without navigating', () => {
    const item = notif({ id: 'n5', link: { surface: 'task', url: '/x' } })
    mockList.mockReturnValue(ok({ notifications: [item], unreadCount: 1 }))
    renderInbox()
    fireEvent.click(screen.getByLabelText('Notifications (1 unread)'))
    fireEvent.click(screen.getByLabelText('Dismiss notification'))
    expect(dismiss).toHaveBeenCalledWith({ id: 'n5' })
    expect(navigate).not.toHaveBeenCalled()
  })

  it('marks all read from the popover header', () => {
    mockList.mockReturnValue(ok({ notifications: [notif({})], unreadCount: 2 }))
    renderInbox()
    fireEvent.click(screen.getByLabelText('Notifications (2 unread)'))
    fireEvent.click(screen.getByText('Mark all read'))
    expect(markAllRead).toHaveBeenCalledWith({ projectId: 'p1' })
  })
})
