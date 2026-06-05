import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SidebarProvider } from '@/components/ui/sidebar'

const mockUseNavigation = vi.fn()
const mockUseAuth = vi.fn()
const mockRpcCall = vi.fn()

vi.mock('../lib/routing', () => ({
  useNavigation: () => mockUseNavigation(),
}))

vi.mock('../hooks/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

vi.mock('../hooks/useProjects', () => ({
  useProjects: () => ({
    data: [{ id: 'playground', root: '/tmp/playground', title: 'Playground', status: 'open' }],
    isLoading: false,
  }),
}))

vi.mock('../hooks/useMetrics', () => ({
  useProjectMetrics: () => ({ data: undefined, isLoading: false }),
}))

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  onNotification: () => () => {},
}))

const { default: Sidebar } = await import('./Sidebar')

function renderSidebar() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SidebarProvider defaultOpen={true} style={{ '--sidebar-width': '220px' } as React.CSSProperties}>
        <Sidebar />
      </SidebarProvider>
    </QueryClientProvider>,
  )
}

describe('Sidebar', () => {
  beforeEach(() => {
    mockRpcCall.mockImplementation(() => Promise.resolve({}))
    mockUseAuth.mockReturnValue({
      isHosted: false,
      bridge: null,
      user: null,
      logout: vi.fn(),
    })
    mockUseNavigation.mockReturnValue({
      activeView: 'dashboard',
      setActiveView: vi.fn(),
      focusedProjectRoot: '/tmp/playground',
      projectId: 'playground',
      setFocusedProjectRoot: vi.fn(),
    })
  })

  it('shows the local daemon status in the sidebar footer', () => {
    mockUseAuth.mockReturnValue({
      isHosted: false,
      bridge: { connected: true, projects: [{ id: 'project-1', name: 'Playground' }] },
      user: { id: 'user-1', name: 'User', email: 'user@example.com', createdAt: '2026-03-25T00:00:00Z' },
      logout: vi.fn(),
    })

    renderSidebar()

    // Daemon status is always visible in the footer (no popover to open).
    expect(screen.getAllByText(/^Local daemon$/).length).toBeGreaterThan(0)
    expect(screen.getByText(/^Connected$/)).toBeInTheDocument()
  })
})
