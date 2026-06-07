import { beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'

const mockUseLocations = vi.fn()
const mockCreateLocation = vi.fn()
const mockProbeLocation = vi.fn()
const mockDeleteLocation = vi.fn()
const mockCreateCredential = vi.fn()

vi.mock('../hooks/useLocations', () => ({
  useLocations: () => mockUseLocations(),
  useCreateLocation: () => ({
    mutateAsync: mockCreateLocation,
    isPending: false,
  }),
  useProbeLocation: () => ({
    mutateAsync: mockProbeLocation,
    isPending: false,
    variables: undefined,
  }),
  useDeleteLocation: () => ({
    mutateAsync: mockDeleteLocation,
    isPending: false,
    variables: undefined,
  }),
}))

vi.mock('../hooks/useCredentials', () => ({
  useCredentialCreate: () => ({
    mutateAsync: mockCreateCredential,
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
          status: 'online',
          ready: true,
          capabilities: [
            { name: 'reachable', status: 'passed' },
            { name: 'fileBrowsing', status: 'passed' },
          ],
          updatedAt: '2026-06-07T10:00:00Z',
        },
        {
          id: 'loc-ssh',
          kind: 'ssh',
          label: 'Work laptop',
          address: { host: 'devbox.local', port: 22, username: 'santino' },
          status: 'not_ready',
          ready: false,
          auth: { mode: 'keyRef', credentialId: 'cred-ssh', hasCredential: true },
          capabilities: [
            { name: 'reachable', status: 'unknown' },
            { name: 'fileBrowsing', status: 'unknown' },
          ],
          lastProbe: { status: 'unknown', message: 'not probed yet' },
        },
      ],
    })
    mockCreateLocation.mockResolvedValue({})
    mockProbeLocation.mockResolvedValue({})
    mockDeleteLocation.mockResolvedValue({})
    mockCreateCredential.mockResolvedValue({ credential: { id: 'cred-created' } })
  })

  it('renders daemon and SSH locations without harness setup controls', () => {
    render(<Locations />)

    expect(screen.getByRole('heading', { name: 'Locations' })).toBeInTheDocument()
    expect(screen.getByText('This machine')).toBeInTheDocument()
    expect(screen.getByText('Daemon machine')).toBeInTheDocument()
    expect(screen.getByText('Work laptop')).toBeInTheDocument()
    expect(screen.getByText('santino@devbox.local:22')).toBeInTheDocument()
    expect(screen.getAllByText('File browsing').length).toBeGreaterThan(0)
    expect(screen.queryByRole('button', { name: /codex login/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /install claude/i })).not.toBeInTheDocument()
  })

  it('creates an SSH location with an existing credential reference', async () => {
    const user = userEvent.setup()
    render(<Locations />)

    await user.type(screen.getByLabelText('Label'), 'Rack mini')
    await user.type(screen.getByLabelText('Host'), 'rack-mini.local')
    await user.type(screen.getByLabelText('Username'), 'santino')
    await user.type(screen.getByLabelText('Credential reference'), 'cred-existing')
    await user.click(screen.getByRole('button', { name: /^add location$/i }))

    await waitFor(() => {
      expect(mockCreateLocation).toHaveBeenCalledWith({
        kind: 'ssh',
        label: 'Rack mini',
        address: { host: 'rack-mini.local', username: 'santino', port: 22 },
        auth: { mode: 'keyRef', credentialId: 'cred-existing' },
      })
    })
    expect(mockCreateCredential).not.toHaveBeenCalled()
  })

  it('can create a password credential before creating an SSH location', async () => {
    const user = userEvent.setup()
    render(<Locations />)

    await user.type(screen.getByLabelText('Host'), '10.0.0.12')
    await user.type(screen.getByLabelText('Username'), 'deploy')
    await user.click(screen.getByRole('combobox', { name: /authentication/i }))
    await user.click(screen.getByRole('option', { name: /create password credential/i }))
    await user.type(screen.getByLabelText('Password'), 'secret-password')
    await user.click(screen.getByRole('button', { name: /^add location$/i }))

    await waitFor(() => {
      expect(mockCreateCredential).toHaveBeenCalledWith({
        kind: 'ssh_password',
        label: 'deploy@10.0.0.12',
        storageKind: 'local_encrypted',
        secrets: { password: 'secret-password' },
      })
      expect(mockCreateLocation).toHaveBeenCalledWith({
        kind: 'ssh',
        label: 'deploy@10.0.0.12',
        address: { host: '10.0.0.12', username: 'deploy', port: 22 },
        auth: { mode: 'keyRef', credentialId: 'cred-created' },
      })
    })
  })

  it('probes and deletes locations through the location hooks', async () => {
    const user = userEvent.setup()
    render(<Locations />)

    await user.click(screen.getByRole('button', { name: /probe work laptop/i }))
    expect(mockProbeLocation).toHaveBeenCalledWith('loc-ssh')

    await user.click(screen.getByRole('button', { name: /delete work laptop/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))
    expect(mockDeleteLocation).toHaveBeenCalledWith('loc-ssh')
  })
})
