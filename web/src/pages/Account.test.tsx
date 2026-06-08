import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockLogout = vi.fn()
const mockUpdateProfile = vi.fn()
const mockCreateAPIKey = vi.fn()
const mockListAPIKeys = vi.fn()
const mockRevokeAPIKey = vi.fn()
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

vi.mock('../lib/authClient', () => ({
  createAPIKey: mockCreateAPIKey,
  listAPIKeys: mockListAPIKeys,
  revokeAPIKey: mockRevokeAPIKey,
}))

vi.mock('sonner', () => ({
  toast: { error: vi.fn(), success: vi.fn(), info: vi.fn() },
}))

const { default: Account } = await import('./Account')
const { useStore } = await import('../lib/store')

function renderAccount() {
  const queryClient = new QueryClient({
    defaultOptions: {
      queries: { retry: false, staleTime: Infinity, refetchOnWindowFocus: false },
    },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <Account />
    </QueryClientProvider>,
  )
}

describe('Account page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    useStore.setState({
      theme: 'dark',
      defaultProjectView: 'dashboard',
    })
    mockUpdateProfile.mockResolvedValue(undefined)
    mockCreateAPIKey.mockResolvedValue({
      key: {
        id: 'key-1',
        name: 'Agen8 MCP key',
        prefix: 'ak_test',
        createdAt: '',
      },
      secret: 'ak_test_secret',
    })
    mockListAPIKeys.mockResolvedValue([
      {
        id: 'key-existing',
        name: 'Existing key',
        prefix: 'ak_existing',
        createdAt: '2026-06-01T12:00:00Z',
        active: true,
      },
    ])
    mockRevokeAPIKey.mockResolvedValue(undefined)
  })

  it('renders signed-in identity and preferences', () => {
    renderAccount()

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
    expect(screen.getByRole('heading', { name: 'MCP access' })).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /generate mcp key/i })).toBeInTheDocument()
    expect(screen.getByRole('heading', { name: 'Agen8 skill' })).toBeInTheDocument()
    expect(screen.getByText('agen8 skill install --harness codex')).toBeInTheDocument()
    expect(screen.getByText('agen8 skill install --harness claude-cli')).toBeInTheDocument()
    expect(screen.queryByText(/auth\.status/i)).not.toBeInTheDocument()
  })

  it('saves profile edits', async () => {
    const user = userEvent.setup()
    renderAccount()

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
    renderAccount()

    await user.click(screen.getByLabelText(/default project view/i))
    await user.click(await screen.findByRole('option', { name: 'Context Map' }))

    expect(useStore.getState().defaultProjectView).toBe('strategy')
    expect(localStorage.getItem('agen8-default-project-view')).toBe('strategy')
  })

  it('generates MCP setup snippets', async () => {
    const user = userEvent.setup()
    renderAccount()

    await user.click(screen.getByRole('button', { name: /generate mcp key/i }))

    expect(mockCreateAPIKey).toHaveBeenCalledWith('Agen8 MCP key')
    expect(await screen.findByText('ak_test_secret')).toBeInTheDocument()
    expect(screen.getByText(/mcpServers/)).toBeInTheDocument()
    expect(screen.getByText(/codex mcp add agen8 --url/)).toBeInTheDocument()
    expect(screen.getByText(/claude mcp add --transport http --scope user agen8/)).toBeInTheDocument()
    expect(screen.getByRole('button', { name: /generate another key/i })).toBeInTheDocument()
  })

  it('lists and revokes existing MCP keys', async () => {
    const user = userEvent.setup()
    renderAccount()

    expect(await screen.findByText('Existing key')).toBeInTheDocument()
    expect(screen.getByText('ak_existing')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /revoke/i }))
    expect(await screen.findByText(/Clients using this key will stop connecting/i)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /revoke key/i }))

    expect(mockRevokeAPIKey).toHaveBeenCalledWith('key-existing')
  })

  it('does not expose password changes before the RPC exists', () => {
    renderAccount()

    expect(screen.queryByLabelText('Current password')).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /change password/i })).not.toBeInTheDocument()
  })
})
