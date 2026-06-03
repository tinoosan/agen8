import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const mockUseLocations = vi.fn()
const mockCreateMutateAsync = vi.fn()
const mockProbeMutateAsync = vi.fn()
const mockInstallCodexMutateAsync = vi.fn()
const mockInstallClaudeMutateAsync = vi.fn()
const mockCodexAuthStatusMutateAsync = vi.fn()
const mockCodexLoginMutateAsync = vi.fn()
const mockClaudeAuthStatusMutateAsync = vi.fn()
const mockClaudeLoginMutateAsync = vi.fn()
const mockClaudeLoginCompleteMutateAsync = vi.fn()
const mockDeleteLocationMutateAsync = vi.fn()
const mockUseCredentials = vi.fn()
const mockCreateCredentialMutateAsync = vi.fn()

vi.mock('../hooks/useLocations', () => ({
  useLocations: () => mockUseLocations(),
  useCreateLocation: () => ({
    mutateAsync: mockCreateMutateAsync,
    isPending: false,
  }),
  useProbeLocation: () => ({
    mutateAsync: mockProbeMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useInstallCodex: () => ({
    mutateAsync: mockInstallCodexMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useInstallClaude: () => ({
    mutateAsync: mockInstallClaudeMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useCodexAuthStatus: () => ({
    mutateAsync: mockCodexAuthStatusMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useCodexLogin: () => ({
    mutateAsync: mockCodexLoginMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useClaudeAuthStatus: () => ({
    mutateAsync: mockClaudeAuthStatusMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useClaudeLogin: () => ({
    mutateAsync: mockClaudeLoginMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useClaudeLoginComplete: () => ({
    mutateAsync: mockClaudeLoginCompleteMutateAsync,
    isPending: false,
    variables: undefined,
  }),
  useDeleteLocation: () => ({
    mutateAsync: mockDeleteLocationMutateAsync,
    isPending: false,
    variables: undefined,
  }),
}))

vi.mock('../hooks/useCredentials', () => ({
  useCredentials: () => mockUseCredentials(),
  useCredentialCreate: () => ({
    mutateAsync: mockCreateCredentialMutateAsync,
    isPending: false,
  }),
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const { default: Locations } = await import('./Locations')

describe('Locations page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseLocations.mockReturnValue({
      isLoading: false,
      data: [
        {
          id: 'local',
          kind: 'local',
          label: 'This machine',
          address: { host: 'santino-macbook' },
          status: 'online',
          ready: true,
          capabilities: [
            { name: 'reachable', status: 'passed' },
            { name: 'fileBrowsing', status: 'passed' },
            { name: 'exec', status: 'passed' },
            { name: 'codex', status: 'passed' },
            { name: 'claude', status: 'passed' },
          ],
          updatedAt: '2026-05-17T12:00:00Z',
        },
      ],
    })
    mockUseCredentials.mockReturnValue({
      isLoading: false,
      data: [],
    })
    mockCreateMutateAsync.mockResolvedValue({})
    mockCreateCredentialMutateAsync.mockResolvedValue({ credential: { id: 'cred_ssh_key' } })
    mockProbeMutateAsync.mockResolvedValue({})
    mockInstallCodexMutateAsync.mockResolvedValue({})
    mockInstallClaudeMutateAsync.mockResolvedValue({})
    mockCodexAuthStatusMutateAsync.mockResolvedValue({ loggedIn: false })
    mockCodexLoginMutateAsync.mockResolvedValue({ output: 'visit https://auth.openai.com/device', loginUrl: 'https://auth.openai.com/device' })
    mockClaudeAuthStatusMutateAsync.mockResolvedValue({ loggedIn: false })
    mockClaudeLoginMutateAsync.mockResolvedValue({ output: 'visit https://claude.com/login', loginUrl: 'https://claude.com/login' })
    mockClaudeLoginCompleteMutateAsync.mockResolvedValue({ output: 'Login successful.' })
    mockDeleteLocationMutateAsync.mockResolvedValue({})
  })

  it('starts Codex login for the local location', async () => {
    const user = userEvent.setup()
    vi.spyOn(window, 'open').mockImplementation(() => null)

    render(<Locations />)
    await user.click(screen.getByRole('button', { name: /codex login/i }))

    expect(mockCodexLoginMutateAsync).toHaveBeenCalledWith('local')
    expect(window.open).toHaveBeenCalledWith('https://auth.openai.com/device', '_blank', 'noopener,noreferrer')
  })

  it('starts Claude login for an ssh location', async () => {
    const user = userEvent.setup()
    mockUseLocations.mockReturnValue({
      isLoading: false,
      data: [
        {
          id: 'loc_ssh',
          kind: 'ssh',
          label: 'Remote',
          address: { host: '10.0.0.2', username: 'santino', port: 22 },
          status: 'not_ready',
          ready: false,
          capabilities: [],
        },
      ],
    })
    vi.spyOn(window, 'open').mockImplementation(() => null)

    render(<Locations />)
    await user.click(screen.getByRole('button', { name: /claude login/i }))

    expect(mockClaudeLoginMutateAsync).toHaveBeenCalledWith('loc_ssh')
    expect(window.open).toHaveBeenCalledWith('https://claude.com/login', '_blank', 'noopener,noreferrer')
  })

  it('submits the Claude authorization code for an ssh location', async () => {
    const user = userEvent.setup()
    mockUseLocations.mockReturnValue({
      isLoading: false,
      data: [
        {
          id: 'loc_ssh',
          kind: 'ssh',
          label: 'Remote',
          address: { host: '10.0.0.2', username: 'santino', port: 22 },
          status: 'not_ready',
          ready: false,
          capabilities: [],
        },
      ],
    })
    vi.spyOn(window, 'open').mockImplementation(() => null)

    render(<Locations />)
    await user.click(screen.getByRole('button', { name: /claude login/i }))
    await user.type(screen.getByLabelText(/authorization code/i), 'oauth-code-123')
    await user.click(screen.getByRole('button', { name: /submit code/i }))

    expect(mockClaudeLoginCompleteMutateAsync).toHaveBeenCalledWith({ locationId: 'loc_ssh', code: 'oauth-code-123' })
  })

  it('renders the local location as the default ready location', () => {
    render(<Locations />)

    expect(screen.getByRole('heading', { name: 'Locations' })).toBeInTheDocument()
    expect(screen.getByText('This machine')).toBeInTheDocument()
    expect(screen.getByText('santino-macbook')).toBeInTheDocument()
    expect(screen.getByText('Default project location')).toBeInTheDocument()
    expect(screen.getByText('Ready')).toBeInTheDocument()
    expect(screen.getByText('Claude')).toBeInTheDocument()
  })

  it('submits an ssh location from the form', async () => {
    const user = userEvent.setup()
    render(<Locations />)

    await user.click(screen.getByRole('button', { name: /ssh location/i }))
    await user.type(screen.getByLabelText('Label'), 'Dev box')
    await user.type(screen.getByLabelText('Host'), '10.0.0.2')
    await user.type(screen.getByLabelText('User'), 'santino')
    await user.type(screen.getByLabelText('Password'), 'secret-password')
    await user.click(screen.getByRole('button', { name: /^add location$/i }))

    expect(mockCreateCredentialMutateAsync).toHaveBeenCalledWith({
      kind: 'ssh_password',
      label: 'santino@10.0.0.2',
      storageKind: 'local_encrypted',
      secrets: { password: 'secret-password' },
    })
    expect(mockCreateMutateAsync).toHaveBeenCalledWith({
      kind: 'ssh',
      label: 'Dev box',
      address: { host: '10.0.0.2', username: 'santino', port: 22 },
      auth: { mode: 'keyRef', credentialId: 'cred_ssh_key' },
    })
  })

  it('offers Codex installation for the local location', async () => {
    const user = userEvent.setup()

    render(<Locations />)
    await user.click(screen.getByRole('button', { name: /install codex/i }))

    expect(mockInstallCodexMutateAsync).toHaveBeenCalledWith('local')
  })

  it('offers codex installation when an ssh location is missing codex', async () => {
    const user = userEvent.setup()
    mockUseLocations.mockReturnValue({
      isLoading: false,
      data: [
        {
          id: 'loc_ssh',
          kind: 'ssh',
          label: 'Remote',
          address: { host: '10.0.0.2', username: 'santino', port: 22 },
          status: 'not_ready',
          ready: false,
          capabilities: [
            { name: 'reachable', status: 'passed' },
            { name: 'fileBrowsing', status: 'passed' },
            { name: 'exec', status: 'passed' },
            { name: 'codex', status: 'failed' },
            { name: 'claude', status: 'unknown' },
          ],
          lastProbe: { failureCode: 'codex_missing', message: 'codex binary was not found on the ssh location' },
        },
      ],
    })

    render(<Locations />)
    await user.click(screen.getByRole('button', { name: /install codex/i }))

    expect(mockInstallCodexMutateAsync).toHaveBeenCalledWith('loc_ssh')
  })

  it('deletes a location after confirmation', async () => {
    const user = userEvent.setup()
    mockUseLocations.mockReturnValue({
      isLoading: false,
      data: [
        {
          id: 'loc_ssh',
          kind: 'ssh',
          label: 'Remote',
          address: { host: '10.0.0.2', username: 'santino', port: 22 },
          status: 'online',
          ready: true,
          capabilities: [],
        },
      ],
    })

    render(<Locations />)
    await user.click(screen.getByRole('button', { name: /delete remote/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    expect(mockDeleteLocationMutateAsync).toHaveBeenCalledWith('loc_ssh')
  })
})
