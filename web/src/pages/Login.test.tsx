import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockNavigate = vi.fn()
vi.mock('wouter', () => ({
  Link: ({ href, children }: { href: string; children: React.ReactNode }) => (
    <a href={href}>{children}</a>
  ),
  useLocation: () => ['/login', mockNavigate],
}))

const mockAuth = {
  login: vi.fn(),
  isLoading: false,
  isHosted: false,
  isAuthenticated: false,
}
vi.mock('../hooks/useAuth', () => ({
  useAuth: () => mockAuth,
}))

const { default: Login } = await import('./Login')

function renderPage() {
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <QueryClientProvider client={client}>
      <Login />
    </QueryClientProvider>,
  )
}

describe('Login page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockAuth.login.mockResolvedValue(undefined)
    mockAuth.isLoading = false
  })

  it('renders the login form', () => {
    renderPage()
    expect(screen.getByRole('heading', { name: /welcome back/i })).toBeInTheDocument()
    expect(screen.getByLabelText(/email/i)).toBeInTheDocument()
    expect(screen.getByLabelText(/password/i)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeInTheDocument()
  })

  it('navigates to / on successful login', async () => {
    const user = userEvent.setup()
    renderPage()

    await user.type(screen.getByLabelText(/email/i), 'test@example.com')
    await user.type(screen.getByLabelText(/password/i), 'password123')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      expect(mockAuth.login).toHaveBeenCalledWith({
        email: 'test@example.com',
        password: 'password123',
      })
      expect(mockNavigate).toHaveBeenCalledWith('/')
    })
  })

  it('shows error message when login fails', async () => {
    const user = userEvent.setup()
    mockAuth.login.mockRejectedValue(new Error('Invalid credentials'))
    renderPage()

    await user.type(screen.getByLabelText(/email/i), 'bad@example.com')
    await user.type(screen.getByLabelText(/password/i), 'wrong')
    await user.click(screen.getByRole('button', { name: /sign in/i }))

    await waitFor(() => {
      expect(screen.getByText('Invalid credentials')).toBeInTheDocument()
    })
  })

  it('disables the button while auth is loading', () => {
    mockAuth.isLoading = true
    renderPage()
    expect(screen.getByRole('button', { name: /sign in/i })).toBeDisabled()
  })

  it('does not expose the deleted register flow', () => {
    renderPage()
    expect(screen.queryByRole('link', { name: /register/i })).not.toBeInTheDocument()
  })
})
