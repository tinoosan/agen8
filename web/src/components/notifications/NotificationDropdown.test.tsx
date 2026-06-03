import { describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import NotificationDropdown from './NotificationDropdown'

const mockUseNotifications = vi.fn()
const mockUseMarkRead = vi.fn()
const mockUseMarkAllRead = vi.fn()
const mockUseDismissNotification = vi.fn()

vi.mock('../../hooks/useNotifications', () => ({
  useNotifications: (...args: unknown[]) => mockUseNotifications(...args),
  useMarkRead: () => mockUseMarkRead(),
  useMarkAllRead: () => mockUseMarkAllRead(),
  useDismissNotification: () => mockUseDismissNotification(),
}))

vi.mock('wouter', () => ({
  useLocation: () => ['/notifications', vi.fn()],
}))

vi.mock('../../lib/routing', () => ({
  useNavigation: () => ({ projectId: 'test-project' }),
}))

describe('NotificationDropdown', () => {
  it('highlights schedule schedule expiry notifications', () => {
    mockUseNotifications.mockReturnValue({
      data: [{
        id: 'n1',
        userId: 'playground',
        source: 'schedule',
        trigger: 'schedule_expiring',
        severity: 'info',
        title: 'Schedule expiring: daily_sync (2 runs left)',
        body: 'Schedule schedule has 2 runs remaining.',
        createdAt: '2026-03-31T09:00:00.000Z',
      }],
      isLoading: false,
    })
    mockUseMarkRead.mockReturnValue({ mutate: vi.fn() })
    mockUseMarkAllRead.mockReturnValue({ mutate: vi.fn() })
    mockUseDismissNotification.mockReturnValue({ mutate: vi.fn() })

    render(<NotificationDropdown userId="playground" onClose={() => {}} />)

    expect(screen.getByText('Schedule expiring')).toBeInTheDocument()
    expect(screen.getByText(/2 runs remaining/i)).toBeInTheDocument()
  })
})
