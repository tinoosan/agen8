import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

vi.mock('wouter', () => ({
  useLocation: () => ['/', vi.fn()],
}))

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({ projectId: 'proj-1', focusedProjectRoot: '/repo' }),
}))

vi.mock('../hooks/useProjects', () => ({
  useProjects: () => ({ data: [] }),
}))

const mockUseNotifications = vi.fn()
const mockUseMarkRead = vi.fn()
const mockUseMarkAllRead = vi.fn()
const mockUseDismissNotification = vi.fn()

vi.mock('../hooks/useNotifications', () => ({
  useNotifications: (...args: unknown[]) => mockUseNotifications(...args),
  useMarkRead: () => mockUseMarkRead(),
  useMarkAllRead: () => mockUseMarkAllRead(),
  useDismissNotification: () => mockUseDismissNotification(),
}))

vi.mock('../components/notifications/NotificationPreferences', () => ({
  default: ({ userId }: { userId: string }) => (
    <div data-testid="notification-preferences">{userId}</div>
  ),
}))

// CustomSelect renders a native select
vi.mock('../components/fields', async (importOriginal) => {
  const mod = await importOriginal<typeof import('../components/fields')>()
  return {
    ...mod,
    CustomSelect: ({ value, onChange, options }: { value: string; onChange: (v: string) => void; options: { value: string; label: string }[] }) => (
      <select value={value} onChange={(e) => onChange(e.target.value)}>
        {options.map((o) => <option key={o.value} value={o.value}>{o.label}</option>)}
      </select>
    ),
  }
})

const { default: Notifications } = await import('./Notifications')

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Notifications />
    </QueryClientProvider>,
  )
}

const sampleNotifications = [
  {
    id: 'n-1',
    title: 'Disk usage critical',
    body: 'Disk is 95% full',
    severity: 'critical' as const,
    source: 'schedule',
    trigger: 'schedule_expiring',
    readAt: null,
    createdAt: new Date().toISOString(),
    link: null,
  },
  {
    id: 'n-2',
    title: 'Task completed',
    body: '',
    severity: 'info' as const,
    source: 'task',
    trigger: 'schedule_completed',
    readAt: new Date().toISOString(),
    createdAt: new Date().toISOString(),
    link: null,
  },
]

describe('Notifications page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseNotifications.mockReturnValue({ data: sampleNotifications, isLoading: false })
    mockUseMarkRead.mockReturnValue({ mutate: vi.fn() })
    mockUseMarkAllRead.mockReturnValue({ mutate: vi.fn() })
    mockUseDismissNotification.mockReturnValue({ mutate: vi.fn() })
  })

  it('renders the Notifications heading', () => {
    renderPage()
    expect(screen.getByText('Notifications')).toBeInTheDocument()
  })

  it('renders notification titles', () => {
    renderPage()
    expect(screen.getByText('Disk usage critical')).toBeInTheDocument()
    expect(screen.getByText('Task completed')).toBeInTheDocument()
  })

  it('renders History and Preferences tabs', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /history/i })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /preferences/i })).toBeInTheDocument()
  })

  it('shows Preferences panel when Preferences tab clicked', async () => {
    const user = userEvent.setup()
    renderPage()
    await user.click(screen.getByRole('button', { name: /preferences/i }))
    await waitFor(() => {
      expect(screen.getByTestId('notification-preferences')).toBeInTheDocument()
    })
  })

  it('shows empty state when no notifications', () => {
    mockUseNotifications.mockReturnValue({ data: [], isLoading: false })
    renderPage()
    expect(screen.getByText(/no notifications match your filters/i)).toBeInTheDocument()
  })

  it('renders mark all read button', () => {
    renderPage()
    expect(screen.getByRole('button', { name: /mark all read/i })).toBeInTheDocument()
  })

  it('renders dismiss buttons for each notification', () => {
    renderPage()
    const dismissButtons = screen.getAllByRole('button', { name: /dismiss/i })
    expect(dismissButtons).toHaveLength(sampleNotifications.length)
  })
})
