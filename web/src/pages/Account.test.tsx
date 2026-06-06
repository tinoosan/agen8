import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const mockLogout = vi.fn()
const mockUpdateProfile = vi.fn()
const mockAuth = {
  user: {
    id: 'user-1',
    name: 'Tino',
    email: 'tino@example.com',
    createdAt: '2026-04-28T12:00:00Z',
  },
  logout: mockLogout,
  updateProfile: mockUpdateProfile,
}

vi.mock('../hooks/useAuth', () => ({
  useAuth: () => mockAuth,
}))

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const { default: Account } = await import('./Account')
const { useStore } = await import('../lib/store')

describe('Account page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    useStore.setState({
      theme: 'dark',
      defaultProjectView: 'dashboard',
    })
    mockUpdateProfile.mockResolvedValue(undefined)
  })

  it('renders signed-in identity and preferences', () => {
    render(<Account />)

    expect(screen.getByRole('heading', { name: 'Settings' })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Account' })).toBeInTheDocument()
    expect(screen.getByLabelText('Name')).toHaveValue('Tino')
    expect(screen.getByLabelText('Email')).toHaveValue('tino@example.com')
    expect(screen.queryByText(/user id/i)).not.toBeInTheDocument()
    expect(screen.queryByText('Connections')).not.toBeInTheDocument()
    expect(screen.queryByText('Local daemon')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /connector access/i })).not.toBeInTheDocument()
    expect(screen.getByText('Current browser')).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Signed in as Tino')).toBeInTheDocument()
    expect(screen.queryByText(/auth\.status/i)).not.toBeInTheDocument()
  })

  it('saves profile edits', async () => {
    const user = userEvent.setup()
    render(<Account />)

    await user.clear(screen.getByLabelText('Name'))
    await user.type(screen.getByLabelText('Name'), 'Santino')
    await user.click(screen.getByRole('button', { name: /save profile/i }))

    expect(mockUpdateProfile).toHaveBeenCalledWith({
      name: 'Santino',
      email: 'tino@example.com',
    })
  })

  it('updates the default project view preference', async () => {
    const user = userEvent.setup()
    render(<Account />)

    await user.click(screen.getByLabelText(/default project view/i))
    await user.click(await screen.findByRole('option', { name: 'Strategy map' }))

    expect(useStore.getState().defaultProjectView).toBe('strategy')
    expect(localStorage.getItem('agen8-default-project-view')).toBe('strategy')
  })

  it('does not expose password changes before the RPC exists', () => {
    render(<Account />)

    expect(screen.queryByLabelText('Current password')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /change password/i })).not.toBeInTheDocument()
  })
})
