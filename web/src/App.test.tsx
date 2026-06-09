import { act } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { fireEvent, render, screen, waitFor } from '@testing-library/react'
import { useStore } from './lib/store'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockUseAuth = vi.fn()

vi.mock('./components/Sidebar', () => ({
  default: () => <div>Sidebar</div>,
}))

vi.mock('./pages/Project', () => ({
  default: () => <div>Project Page</div>,
}))

vi.mock('./pages/Login', () => ({
  default: () => <div>Login Page</div>,
}))

vi.mock('./pages/Dashboard', () => ({
  default: () => <div>Dashboard Page</div>,
}))

vi.mock('./pages/Pulse', () => ({
  default: () => <div>Pulse Page</div>,
}))

vi.mock('./pages/StrategyMap', () => ({
  default: () => {
    throw new Error('strategy-map-route-crash')
  },
}))

// Mock useProjects to provide project data for routing resolution
vi.mock('./hooks/useProjects', () => ({
  useProjects: () => ({
    data: [
      { id: 'myapp', root: '/repo', title: 'myapp', status: 'open' },
    ],
    isLoading: false,
  }),
}))

vi.mock('./hooks/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

const { default: App } = await import('./App')

function resetStore() {
  act(() => {
    useStore.setState({
      artifactsOpen: false,
      strategySearchOpen: false,
      theme: 'dark',
      lastDarkTheme: 'dark',
      lastLightTheme: 'light',
      defaultProjectView: 'dashboard',
      fontFamily: 'inter',
      fontScale: 16,
    })
  })
}

function renderWithRouter(path: string = '/') {
  const location = memoryLocation({ path, record: true })
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, refetchOnWindowFocus: false },
    },
  })

  const view = render(
    <QueryClientProvider client={queryClient}>
      <Router hook={location.hook}>
        <App />
      </Router>
    </QueryClientProvider>
  )

  return { ...view, history: location.history }
}

describe('App', () => {
  beforeEach(() => {
    resetStore()
    document.documentElement.removeAttribute('data-theme')
    mockUseAuth.mockReturnValue({
      isLoading: false,
      isHosted: false,
      isAuthenticated: true,
      status: { enabled: true, hostedMode: false, authenticated: true },
    })
  })

  it('renders the project page at root path', async () => {
    renderWithRouter('/')
    expect(await screen.findByText('Project Page')).toBeInTheDocument()
  })

  it('renders the pulse page at the pulse route', async () => {
    renderWithRouter('/project/myapp/pulse')
    expect(await screen.findByText('Pulse Page')).toBeInTheDocument()
  })

  it('redirects the legacy metrics and activity routes to pulse', async () => {
    const metrics = renderWithRouter('/project/myapp/metrics')
    expect(await screen.findByText('Pulse Page')).toBeInTheDocument()
    expect(metrics.history.at(-1)).toBe('/project/myapp/pulse')
    metrics.unmount()

    const activity = renderWithRouter('/project/myapp/activity')
    expect(await screen.findByText('Pulse Page')).toBeInTheDocument()
    expect(activity.history.at(-1)).toBe('/project/myapp/pulse')
  })

  it('shows page crash fallback when a route component throws', async () => {
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      renderWithRouter('/project/myapp/strategy')
      expect(await screen.findByText('This page crashed')).toBeInTheDocument()
      expect(screen.getByText('Try again')).toBeInTheDocument()
    } finally {
      consoleErrorSpy.mockRestore()
    }
  })

  it('does not show the mobile search button off the context map', async () => {
    // The only search worth a touch entry point is the context-map node
    // search; off the map there's no global search, so the button is hidden.
    renderWithRouter('/')

    expect(await screen.findByText('Project Page')).toBeInTheDocument()
    expect(screen.queryByLabelText('Open search')).not.toBeInTheDocument()
  })

  it('mobile search button opens the context-map node search on the strategy route', async () => {
    // The StrategyMap page mock throws (see top of file), so the route renders
    // the crash boundary — but the mobile top bar lives outside it and the
    // button only depends on the resolved view being "strategy".
    const consoleErrorSpy = vi.spyOn(console, 'error').mockImplementation(() => {})
    try {
      renderWithRouter('/project/myapp/strategy')

      fireEvent.click(await screen.findByLabelText('Open search'))
      expect(useStore.getState().strategySearchOpen).toBe(true)
    } finally {
      consoleErrorSpy.mockRestore()
    }
  })

  it('mobile search button on the dashboard opens node search in place without navigating', async () => {
    // The dashboard renders the node-search modal off the shared
    // strategySearchOpen flag, so the mobile button just flips the flag — it
    // must not navigate away to the context map.
    const { history } = renderWithRouter('/project/myapp/dashboard')

    fireEvent.click(await screen.findByLabelText('Open search'))
    expect(useStore.getState().strategySearchOpen).toBe(true)
    expect(history.at(-1)).toBe('/project/myapp/dashboard')
  })

  it('applies the selected theme to the document root', async () => {
    act(() => {
      useStore.setState({ theme: 'dim' })
    })

    renderWithRouter('/')

    await waitFor(() => {
      expect(document.documentElement.getAttribute('data-theme')).toBe('dim')
    })
  })

  it('syncs signed-in user preferences through the account profile', async () => {
    const updateProfile = vi.fn().mockResolvedValue(undefined)
    mockUseAuth.mockReturnValue({
      isLoading: false,
      isHosted: false,
      isAuthenticated: true,
      status: { enabled: true, hostedMode: false, authenticated: true },
      user: {
        id: 'user-1',
        email: 'tino@example.com',
        name: 'Tino',
        createdAt: '2026-04-28T12:00:00Z',
        preferences: {
          theme: 'rose',
          lastDarkTheme: 'rose',
          lastLightTheme: 'sepia',
          defaultProjectView: 'strategy',
          fontFamily: 'lora',
          fontScale: 18,
        },
      },
      updateProfile,
    })
    renderWithRouter('/')

    await waitFor(() => {
      expect(useStore.getState().theme).toBe('rose')
    })
    expect(useStore.getState().fontFamily).toBe('lora')
    expect(useStore.getState().fontScale).toBe(18)
    expect(updateProfile).not.toHaveBeenCalled()

    act(() => {
      useStore.getState().setTheme('forest')
    })

    await waitFor(() => {
      expect(updateProfile).toHaveBeenCalledWith({
        preferences: expect.objectContaining({
          theme: 'forest',
          lastDarkTheme: 'forest',
          lastLightTheme: 'sepia',
          defaultProjectView: 'strategy',
          fontFamily: 'lora',
          fontScale: 18,
        }),
      })
    })
  })

  it('renders login routes without the sidebar shell when unauthenticated', async () => {
    mockUseAuth.mockReturnValue({
      isLoading: false,
      isHosted: false,
      isAuthenticated: false,
      status: { enabled: true, hostedMode: false, authenticated: false },
    })

    renderWithRouter('/login')
    expect(await screen.findByText('Login Page')).toBeInTheDocument()
    expect(screen.queryByText('Sidebar')).not.toBeInTheDocument()
  })

  it('renders the login route while auth status is still loading', async () => {
    mockUseAuth.mockReturnValue({
      isLoading: true,
      isHosted: false,
      isAuthenticated: false,
      status: undefined,
    })

    renderWithRouter('/login')
    expect(await screen.findByText('Login Page')).toBeInTheDocument()
    expect(screen.queryByText('Sidebar')).not.toBeInTheDocument()
  })

  it('redirects protected routes to login when unauthenticated in local mode', async () => {
    mockUseAuth.mockReturnValue({
      isLoading: false,
      isHosted: false,
      isAuthenticated: false,
      status: { enabled: true, hostedMode: false, authenticated: false },
    })

    renderWithRouter('/')
    expect(await screen.findByText('Login Page')).toBeInTheDocument()
  })

  it('redirects protected routes to login when unauthenticated in hosted mode', async () => {
    mockUseAuth.mockReturnValue({
      isLoading: false,
      isHosted: true,
      isAuthenticated: false,
      status: { enabled: true, hostedMode: true, authenticated: false },
    })

    renderWithRouter('/')
    expect(await screen.findByText('Login Page')).toBeInTheDocument()
  })
})
