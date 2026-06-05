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

vi.mock('./pages/StrategyMap', () => ({
  default: () => {
    throw new Error('strategy-map-route-crash')
  },
}))

vi.mock('./components/CommandPalette', () => ({
  default: () => <div>Command Palette</div>,
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
      paletteOpen: false,
      theme: 'dark',
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

  it('redirects removed metrics route to the dashboard', async () => {
    renderWithRouter('/project/myapp/metrics')
    expect(await screen.findByText('Dashboard Page')).toBeInTheDocument()
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

  it('opens and closes the command palette from keyboard shortcuts', async () => {
    renderWithRouter('/')

    fireEvent.keyDown(window, { ctrlKey: true, key: 'k' })
    expect(await screen.findByText('Command Palette')).toBeInTheDocument()

    fireEvent.keyDown(window, { key: 'Escape' })
    await waitFor(() => {
      expect(screen.queryByText('Command Palette')).not.toBeInTheDocument()
    })
  })

  it('opens the command palette with Cmd/Ctrl+Shift+P', async () => {
    renderWithRouter('/')

    fireEvent.keyDown(window, { ctrlKey: true, shiftKey: true, key: 'P' })
    expect(await screen.findByText('Command Palette')).toBeInTheDocument()
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
