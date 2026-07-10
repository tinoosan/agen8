import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { createTestQueryClient } from '../test/test-utils'

const mockUseProjects = vi.fn()
const mockSetFocusedProjectRoot = vi.fn()
const mockUseLocations = vi.fn()
const mockRpcCall = vi.fn()

vi.mock('../hooks/useProjects', () => ({
  useProjects: () => mockUseProjects(),
}))

vi.mock('../hooks/useLocations', () => ({
  useLocations: () => mockUseLocations(),
}))

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
}))

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({
    projectId: null,
    focusedProjectRoot: null,
    activeView: 'project' as const,
    projectLoading: false,
    setFocusedProjectRoot: mockSetFocusedProjectRoot,
    setActiveView: vi.fn(),
  }),
}))

const { default: Project } = await import('./Project')

function renderProjectPage() {
  const queryClient = createTestQueryClient()
  const { hook } = memoryLocation({ path: '/' })

  return render(
    <QueryClientProvider client={queryClient}>
      <Router hook={hook}>
        <Project />
      </Router>
    </QueryClientProvider>,
  )
}

describe('Project page', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    mockUseProjects.mockReturnValue({
      isLoading: false,
      isError: false,
      error: null,
      refetch: vi.fn(),
      data: [
        { id: 'alpha', root: '/repo/alpha', title: 'alpha', status: 'open', createdAt: '2026-03-01T00:00:00Z' },
        { id: 'beta', root: '/repo/beta', title: 'beta', status: 'open', createdAt: '2026-03-02T00:00:00Z' },
      ],
    })
    mockUseLocations.mockReturnValue({
      isLoading: false,
      data: [
        {
          id: 'local',
          kind: 'local',
          label: 'This machine',
          status: 'online',
          ready: true,
          capabilities: [],
        },
      ],
    })
    mockRpcCall.mockImplementation((method: string) => {
      if (method === 'location.fs.listDir') {
        return Promise.resolve({ entries: [{ name: 'repo', path: '/Users/tino/repo', type: 'directory' }] })
      }
      if (method === 'project.create') {
        return Promise.resolve({
          project: {
            id: 'proj_repo',
            locationId: 'local',
            root: '/Users/tino/repo',
            title: 'repo',
            status: 'open',
          },
        })
      }
      return Promise.resolve({})
    })
  })

  it('renders project cards for each registered project', () => {
    renderProjectPage()

    expect(screen.getByText('alpha')).toBeInTheDocument()
    expect(screen.getByText('beta')).toBeInTheDocument()
  })

  it('renders a retryable error instead of a false empty state', async () => {
    const user = userEvent.setup()
    const refetch = vi.fn()
    mockUseProjects.mockReturnValue({
      isLoading: false,
      isError: true,
      error: new Error('project list unavailable'),
      refetch,
      data: undefined,
    })

    renderProjectPage()

    expect(screen.getByText('Projects could not be loaded')).toBeInTheDocument()
    expect(screen.queryByText('Create your first project')).not.toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: 'Try again' }))
    expect(refetch).toHaveBeenCalledOnce()
  })

  it('repairs Claude MCP configuration for an existing project', async () => {
    const user = userEvent.setup()
    mockRpcCall.mockImplementation((method: string) => {
      if (method === 'project.claudeMCP.configure') {
        return Promise.resolve({ projectId: 'alpha', installed: true, serverName: 'agen8' })
      }
      return Promise.resolve({})
    })

    renderProjectPage()

    const actions = screen.getAllByRole('button', { name: /project actions/i })
    await user.click(actions[0])
    await user.click(screen.getByRole('menuitem', { name: /configure claude mcp/i }))
    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('project.claudeMCP.configure', { projectId: 'alpha' })
    })
  })

  it('shows a one-time client command for hosted existing-project setup', async () => {
    const user = userEvent.setup()
    const command = "agen8 client setup --harness claude --url 'https://agen8.example.com' --token 'ak_once'"
    mockRpcCall.mockImplementation((method: string) => {
      if (method === 'project.claudeMCP.configure') {
        return Promise.resolve({
          projectId: 'alpha',
          installed: false,
          requiresClientAction: true,
          clientSetupCommand: command,
        })
      }
      return Promise.resolve({})
    })

    renderProjectPage()
    await user.click(screen.getAllByRole('button', { name: /project actions/i })[0])
    await user.click(screen.getByRole('menuitem', { name: /configure claude mcp/i }))

    expect(await screen.findByRole('dialog')).toHaveTextContent('Finish Claude setup')
    expect(screen.getByText(command)).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /done/i }))
    expect(screen.queryByText(command)).not.toBeInTheDocument()
  })

  it('uses the ready local location by default when creating the first project', async () => {
    const user = userEvent.setup()
    mockUseProjects.mockReturnValue({ isLoading: false, data: [] })

    renderProjectPage()

    // Empty state shows "New project" CTA
    expect(screen.getByText('Create your first project')).toBeInTheDocument()
    await user.click(screen.getByRole('button', { name: /new project/i }))

    // Dialog opens on directory step (location step skipped with 1 location)
    expect(await screen.findByRole('dialog')).toBeInTheDocument()

    // Browse and select a directory
    await user.click(await screen.findByText('repo'))
    await user.click(screen.getByRole('button', { name: /select folder/i }))

    // Advances to details step — click create
    await user.click(screen.getByRole('button', { name: /create project/i }))

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('project.create', {
        locationId: 'local',
        root: '/Users/tino/repo',
        title: undefined,
      })
    })
  })

  it('keeps the hosted setup command visible after creating a project', async () => {
    const user = userEvent.setup()
    const command = "agen8 client setup --harness claude --url 'https://agen8.example.com' --token 'ak_once'"
    mockUseProjects.mockReturnValue({ isLoading: false, data: [] })
    mockRpcCall.mockImplementation((method: string) => {
      if (method === 'location.fs.listDir') {
        return Promise.resolve({ entries: [{ name: 'repo', path: '/Users/tino/repo', type: 'directory' }] })
      }
      if (method === 'project.create') {
        return Promise.resolve({
          project: {
            id: 'proj_repo',
            locationId: 'local',
            root: '/Users/tino/repo',
            title: 'repo',
            status: 'open',
          },
          setup: {
            attempted: false,
            hooksInstalled: false,
            claudeMcpConfigured: false,
            requiresClientAction: true,
            clientSetupCommand: command,
          },
        })
      }
      return Promise.resolve({})
    })

    renderProjectPage()
    await user.click(screen.getByRole('button', { name: /new project/i }))
    await user.click(await screen.findByText('repo'))
    await user.click(screen.getByRole('button', { name: /select folder/i }))
    await user.click(screen.getByRole('button', { name: /create project/i }))

    expect(await screen.findByText(command)).toBeInTheDocument()
    expect(screen.getByRole('dialog')).toHaveTextContent('Finish Claude setup')
    await user.click(screen.getByRole('button', { name: /done/i }))
    expect(screen.queryByText(command)).not.toBeInTheDocument()
  })

  it('deletes an archived project from the project table', async () => {
    const user = userEvent.setup()
    mockUseProjects.mockReturnValue({
      isLoading: false,
      data: [
        { id: 'old', root: '/repo/old', title: 'old', status: 'archived', createdAt: '2026-03-01T00:00:00Z' },
      ],
    })

    renderProjectPage()

    // Archived projects are visible by default (filter starts on "All")
    expect(screen.getByText('old')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /project actions/i }))
    await user.click(screen.getByRole('menuitem', { name: /delete project/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    expect(mockRpcCall).toHaveBeenCalledWith('project.delete', { projectId: 'old' })
  })

  it('offers delete directly for an active project and archives before deleting', async () => {
    const user = userEvent.setup()
    mockUseProjects.mockReturnValue({
      isLoading: false,
      data: [
        { id: 'active', root: '/repo/active', title: 'active', status: 'open', createdAt: '2026-03-01T00:00:00Z' },
      ],
    })

    renderProjectPage()

    await user.click(screen.getByRole('button', { name: /project actions/i }))
    expect(screen.getByRole('menuitem', { name: /archive project/i })).toBeInTheDocument()
    await user.click(screen.getByRole('menuitem', { name: /delete project/i }))
    await user.click(screen.getByRole('button', { name: /^delete$/i }))

    expect(mockRpcCall).toHaveBeenCalledWith('project.archive', { projectId: 'active' })
    expect(mockRpcCall).toHaveBeenCalledWith('project.delete', { projectId: 'active' })
  })
})
