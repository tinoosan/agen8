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

  it('shows the signed-in identity in the sidebar footer', () => {
    mockUseAuth.mockReturnValue({
      isHosted: false,
      user: { id: 'user-1', name: 'User', email: 'user@example.com', createdAt: '2026-03-25T00:00:00Z' },
      logout: vi.fn(),
    })

    renderSidebar()

    expect(screen.getByText(/^User$/)).toBeInTheDocument()
    expect(screen.queryByText(/^Local daemon$/)).not.toBeInTheDocument()
  })

  it('links daemon-level locations from the sidebar footer', () => {
    renderSidebar()

    expect(screen.getByRole('button', { name: /locations/i })).toBeInTheDocument()
  })
})
