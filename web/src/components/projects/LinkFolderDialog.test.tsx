import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { render, screen, within } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import type { ReactElement } from 'react'
import type { Project } from '../../lib/types'
import type { LinkTokenSummary } from '../../lib/projectClient'
import { createWrapper } from '../../test/test-utils'

const mockCreateLinkToken = vi.fn()
const mockListLinkTokens = vi.fn()
const mockRevokeLinkToken = vi.fn()

vi.mock('../../lib/projectClient', () => ({
  createLinkToken: (...args: unknown[]) => mockCreateLinkToken(...args),
  listLinkTokens: (...args: unknown[]) => mockListLinkTokens(...args),
  revokeLinkToken: (...args: unknown[]) => mockRevokeLinkToken(...args),
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

// Each test gets a fresh QueryClient provider because the dialog now lists a
// project's link tokens through a useQuery-backed hook.
function renderDialog(ui: ReactElement) {
  const { Wrapper } = createWrapper()
  return render(ui, { wrapper: Wrapper })
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
    // Default: project has no existing tokens. Individual tests override this.
    mockListLinkTokens.mockResolvedValue([])
    mockRevokeLinkToken.mockResolvedValue(undefined)
  })

  afterEach(() => {
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker
  })

  it('reveals the minted token once, only after an explicit generate', async () => {
    const user = userEvent.setup()
    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)

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
    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    expect(await screen.findByText('.agen8/workspace.json')).toBeInTheDocument()
    expect(screen.getByText('.agen8/token')).toBeInTheDocument()
    expect(screen.getByText('.agen8/.gitignore')).toBeInTheDocument()
    // The committed pointer carries this server's origin.
    expect(screen.getByText(new RegExp(`"server_url": "${window.location.origin}"`))).toBeInTheDocument()
  })

  it('does not print a project root in the placement instructions', async () => {
    const user = userEvent.setup()
    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))
    await screen.findByTestId('link-token-value')

    // The root is workspace-sourced and not known here, so the stale
    // "/Users/tino/repo/.agen8/" path must never appear.
    expect(screen.queryByText(/\/Users\/tino\/repo/)).not.toBeInTheDocument()
  })

  it('surfaces the copy/paste fallback when the browser cannot write folders', async () => {
    const user = userEvent.setup()
    delete (window as unknown as { showDirectoryPicker?: unknown }).showDirectoryPicker

    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    await screen.findByTestId('link-token-value')
    expect(screen.getByText(/can't write folders directly/i)).toBeInTheDocument()
    expect(screen.queryByRole('button', { name: /save to folder/i })).not.toBeInTheDocument()
  })

  it('offers Save to folder when the File System Access API is available', async () => {
    const user = userEvent.setup()
    setDirectoryPicker(vi.fn())

    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    await screen.findByTestId('link-token-value')
    expect(screen.getByRole('button', { name: /save to folder/i })).toBeInTheDocument()
    expect(screen.queryByText(/can't write folders directly/i)).not.toBeInTheDocument()
  })

  it('shows the mint error loudly and does not reveal a token', async () => {
    const user = userEvent.setup()
    mockCreateLinkToken.mockRejectedValueOnce(new Error('project repo-abc123 is not owned by caller'))

    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await user.click(screen.getByRole('button', { name: /generate link token/i }))

    expect(await screen.findByText(/not owned by caller/i)).toBeInTheDocument()
    expect(screen.queryByTestId('link-token-value')).not.toBeInTheDocument()
  })

  it('reports an unlinked project when there are no tokens', async () => {
    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    expect(await screen.findByText(/isn't linked to a folder/i)).toBeInTheDocument()
    expect(mockListLinkTokens).toHaveBeenCalledWith('repo-abc123')
  })

  it('lists existing tokens with their status', async () => {
    const summaries: LinkTokenSummary[] = [
      { id: 'tok-1', prefix: 'wlt_aaa', projectId: 'repo-abc123', label: 'laptop', status: 'active', createdAt: new Date().toISOString() },
      { id: 'tok-2', prefix: 'wlt_bbb', projectId: 'repo-abc123', status: 'revoked', createdAt: new Date().toISOString() },
    ]
    mockListLinkTokens.mockResolvedValue(summaries)

    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)

    expect(await screen.findByText('wlt_aaa…')).toBeInTheDocument()
    expect(screen.getByText('laptop')).toBeInTheDocument()
    expect(screen.getByText('Active')).toBeInTheDocument()
    expect(screen.getByText('Revoked')).toBeInTheDocument()
  })

  it('only offers Revoke for active tokens', async () => {
    const summaries: LinkTokenSummary[] = [
      { id: 'tok-1', prefix: 'wlt_aaa', projectId: 'repo-abc123', status: 'active', createdAt: new Date().toISOString() },
      { id: 'tok-2', prefix: 'wlt_bbb', projectId: 'repo-abc123', status: 'revoked', createdAt: new Date().toISOString() },
    ]
    mockListLinkTokens.mockResolvedValue(summaries)

    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await screen.findByText('wlt_aaa…')

    // One active token -> exactly one Revoke button.
    expect(screen.getAllByRole('button', { name: /revoke/i })).toHaveLength(1)
  })

  it('confirms then revokes an active token', async () => {
    const user = userEvent.setup()
    const summaries: LinkTokenSummary[] = [
      { id: 'tok-1', prefix: 'wlt_aaa', projectId: 'repo-abc123', status: 'active', createdAt: new Date().toISOString() },
    ]
    mockListLinkTokens.mockResolvedValue(summaries)

    renderDialog(<LinkFolderDialog project={project} onClose={() => {}} />)
    await screen.findByText('wlt_aaa…')

    // Clicking Revoke opens a confirmation rather than firing immediately.
    await user.click(screen.getByRole('button', { name: /^revoke$/i }))
    const confirm = await screen.findByRole('alertdialog')
    expect(within(confirm).getByText(/Revoking wlt_aaa/i)).toBeInTheDocument()
    expect(mockRevokeLinkToken).not.toHaveBeenCalled()

    await user.click(within(confirm).getByRole('button', { name: /revoke token/i }))
    expect(mockRevokeLinkToken).toHaveBeenCalledWith('repo-abc123', 'tok-1')
  })
})
