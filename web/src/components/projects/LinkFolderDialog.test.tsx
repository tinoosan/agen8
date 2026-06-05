import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { Project } from '../../lib/types'

const mockCreateLinkToken = vi.fn()

vi.mock('../../lib/projectClient', () => ({
  createLinkToken: (...args: unknown[]) => mockCreateLinkToken(...args),
}))

vi.mock('sonner', () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}))

const { default: LinkFolderDialog } = await import('./LinkFolderDialog')

const project: Project = {
  id: 'repo-abc123',
  locationId: 'local',
  root: '/Users/tino/repo',
  title: 'repo',
  status: 'open',
}

function setDirectoryPicker(fn: unknown) {
  ;(window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker = fn
}

describe('LinkFolderDialog', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker
    mockCreateLinkToken.mockResolvedValue({
      id: 'link_token_1',
      prefix: 'wlt_minted_',
      token: 'wlt_minted_secret',
      projectId: 'repo-abc123',
    })
  })

  afterEach(() => {
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker
  })

  it('reveals the minted token once, only after an explicit generate', async () => {
    const user = userEvent.setup()
    render(<LinkFolderDialog project={project} onClose={() => {}} />)

    // The token is not present before minting.
    expect(screen.queryByTestId('link-token-value')).not.toBeInTheDocument()
    expect(mockCreateLinkToken).not.toHaveBeenCalled()

    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    // After minting, the raw token is revealed with a "shown once" warning.
    const reveal = await screen.findByTestId('link-token-value')
    expect(reveal).toHaveTextContent('wlt_minted_secret')
    expect(screen.getByText(/won't be shown again/i)).toBeInTheDocument()
    expect(mockCreateLinkToken).toHaveBeenCalledWith('repo-abc123')
  })

  it('renders the three marker files with the page origin in the pointer', async () => {
    const user = userEvent.setup()
    render(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    expect(await screen.findByText('.agen8/workspace.json')).toBeInTheDocument()
    expect(screen.getByText('.agen8/token')).toBeInTheDocument()
    expect(screen.getByText('.agen8/.gitignore')).toBeInTheDocument()
    // The committed pointer carries this server's origin.
    expect(screen.getByText(new RegExp(`"server_url": "${window.location.origin}"`))).toBeInTheDocument()
  })

  it('surfaces the copy/paste fallback when the browser cannot write folders', async () => {
    const user = userEvent.setup()
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker

    render(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    await screen.findByTestId('link-token-value')
    expect(screen.getByText(/can't write folders directly/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save to folder/i })).not.toBeInTheDocument()
  })

  it('offers Save to folder when the File System Access API is available', async () => {
    const user = userEvent.setup()
    setDirectoryPicker(vi.fn())

    render(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    await screen.findByTestId('link-token-value')
    expect(screen.getByRole('button', { name: /save to folder/i })).toBeInTheDocument()
    expect(screen.queryByText(/can't write folders directly/i)).not.toBeInTheDocument()
  })

  it('shows the mint error loudly and does not reveal a token', async () => {
    const user = userEvent.setup()
    mockCreateLinkToken.mockRejectedValueOnce(new Error('project repo-abc123 is not owned by caller'))

    render(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    expect(await screen.findByText(/not owned by caller/i)).toBeInTheDocument()
    expect(screen.queryByTestId('link-token-value')).not.toBeInTheDocument()
  })
})
