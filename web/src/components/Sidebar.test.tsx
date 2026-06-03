import React from 'react'
import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, waitFor, fireEvent } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { SidebarProvider } from '@/components/ui/sidebar'

const mockUseNavigation = vi.fn()
const mockUseAuth = vi.fn()
const mockRpcCall = vi.fn()
const mockSpaceUpdateMutate = vi.fn()
const mockUseSpaceUpdate = vi.fn()
const mockUseSpaceDelete = vi.fn()
const mockUseSpaceList = vi.fn()
const mockUseSpaceMemberList = vi.fn()
const mockUseSpaceMemberRegister = vi.fn()
const mockUseSpaceMemberRemove = vi.fn()

vi.mock('../lib/routing', () => ({
  useNavigation: () => mockUseNavigation(),
}))

vi.mock('../hooks/useAuth', () => ({
  useAuth: () => mockUseAuth(),
}))

vi.mock('../hooks/useSpace', () => ({
  useSpaceUpdate: () => mockUseSpaceUpdate(),
  useSpaceDelete: () => mockUseSpaceDelete(),
  useSpaceList: (...args: unknown[]) => mockUseSpaceList(...args),
  useSpaceMemberList: (...args: unknown[]) => mockUseSpaceMemberList(...args),
  useSpaceMemberRegister: () => mockUseSpaceMemberRegister(),
  useSpaceMemberRemove: () => mockUseSpaceMemberRemove(),
}))

vi.mock('../hooks/useProjects', () => ({
  useProjects: () => ({
    data: [{ id: 'playground', root: '/tmp/playground', title: 'Playground', status: 'open' }],
    isLoading: false,
  }),
}))

vi.mock('../hooks/useMetrics', () => ({
  useProjectMetrics: () => ({ data: undefined, isLoading: false }),
}))

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  onNotification: () => () => {},
}))

const { default: Sidebar } = await import('./Sidebar')

function renderSidebar() {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <SidebarProvider defaultOpen={true} style={{ '--sidebar-width': '220px' } as React.CSSProperties}>
        <Sidebar />
      </SidebarProvider>
    </QueryClientProvider>,
  )
}

describe('Sidebar', () => {
  beforeEach(() => {
    mockRpcCall.mockImplementation(() => Promise.resolve({}))
    mockUseAuth.mockReturnValue({
      isHosted: false,
      bridge: null,
      user: null,
      logout: vi.fn(),
    })
    mockUseNavigation.mockReturnValue({
      activeView: 'dashboard',
      setActiveView: vi.fn(),
      focusedProjectRoot: '/tmp/playground',
      projectId: 'playground',
      focusedSpaceId: null,
      setFocusedSpaceId: vi.fn(),
      setFocusedProjectRoot: vi.fn(),
    })
    mockUseSpaceMemberList.mockReturnValue({ data: [], isLoading: false })
    // Default: no mutation pending
    mockSpaceUpdateMutate.mockReset()
    mockUseSpaceUpdate.mockReturnValue({ mutate: mockSpaceUpdateMutate, isPending: false })
    mockUseSpaceDelete.mockReturnValue({ mutate: vi.fn(), isPending: false })
    mockUseSpaceMemberRegister.mockReturnValue({ mutate: vi.fn(), isPending: false })
    mockUseSpaceMemberRemove.mockReturnValue({ mutate: vi.fn(), isPending: false })
    mockUseSpaceList.mockReturnValue({
      data: [{ id: 'space-finance', projectId: 'playground', title: 'Finance', status: 'open' }],
      isLoading: false,
    })
  })

  it('lists spaces within the focused project scope', () => {
    renderSidebar()

    expect(mockUseSpaceList).toHaveBeenCalledWith({ projectId: 'playground', status: 'open', enabled: true })
    expect(screen.getAllByText('Finance').length).toBeGreaterThanOrEqual(1)
  })

  it('renders open spaces in the space list', () => {
    mockUseNavigation.mockReturnValue({
      activeView: 'dashboard',
      setActiveView: vi.fn(),
      focusedProjectRoot: '/tmp/playground',
      projectId: 'playground',
      focusedSpaceId: null,
      setFocusedSpaceId: vi.fn(),
      setFocusedProjectRoot: vi.fn(),
    })
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-other', title: 'Research', status: 'open' }], isLoading: false })

    renderSidebar()
    expect(screen.getByTestId('space-row-space-other')).toBeInTheDocument()
    expect(screen.getByText('Research')).toBeInTheDocument()
  })

  it('creates a space via the section header + button', async () => {
    const user = userEvent.setup()
    const setFocusedSpaceIdMock = vi.fn()
    mockUseNavigation.mockReturnValue({
      activeView: 'dashboard',
      setActiveView: vi.fn(),
      focusedProjectRoot: '/tmp/playground',
      projectId: 'playground',
      focusedSpaceId: null,
      setFocusedSpaceId: setFocusedSpaceIdMock,
      setFocusedProjectRoot: vi.fn(),
    })
    mockUseSpaceList.mockReturnValue({
      data: [{ id: 'space-existing', title: 'Existing space', status: 'open' }],
      isLoading: false,
    })
    mockRpcCall.mockImplementation((method: string) => {
      if (method === 'space.create') {
        return Promise.resolve({ space: { id: 'space-new-123', projectId: 'playground', title: 'Untitled space' } })
      }
      if (method === 'notifications.unreadCount') {
        return Promise.resolve({ count: 0 })
      }
      return Promise.resolve({})
    })

    renderSidebar()

    // The "New space" button is an aria-labeled + icon in the section header
    await user.click(screen.getByRole('button', { name: 'New space' }))

    await waitFor(() => {
      expect(mockRpcCall).toHaveBeenCalledWith('space.create', { projectId: 'playground' })
    })
    expect(setFocusedSpaceIdMock).toHaveBeenCalledWith('space-new-123')
  })


  it('shows the local daemon status inside the account chip popover', () => {
    mockUseAuth.mockReturnValue({
      isHosted: false,
      bridge: { connected: true, projects: [{ id: 'project-1', name: 'Playground' }] },
      user: { id: 'user-1', name: 'User', email: 'user@example.com', createdAt: '2026-03-25T00:00:00Z' },
      logout: vi.fn(),
    })

    renderSidebar()

    // Daemon status lives in the unified AccountChip popover. Click
    // the user identity chip to open it.
    const accountChip = screen.getByRole('button', { name: /^User$/i })
    fireEvent.click(accountChip)
    expect(screen.getAllByText(/^Local daemon$/).length).toBeGreaterThan(0)
    expect(screen.getByText(/^Connected$/)).toBeInTheDocument()
  })

  it('renames a space from the themed modal and slugifies the entered name', async () => {
    const user = userEvent.setup()
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-abc12345', title: 'Finance', status: 'open' }], isLoading: false })

    renderSidebar()

    await user.click(screen.getByTestId('space-actions-space-abc12345'))
    await user.click(await screen.findByTestId('rename-space-space-abc12345'))

    expect(screen.getByRole('dialog')).toBeInTheDocument()
    const input = screen.getByLabelText('Space name')
    await user.clear(input)
    await user.type(input, 'Research TNXP 2026!!!')
    expect(screen.getByText(/Identifier preview:/i)).toHaveTextContent('research-tnxp-2026')

    await user.click(screen.getByRole('button', { name: 'Save name' }))

    expect(mockSpaceUpdateMutate).toHaveBeenCalledWith(
      { spaceId: 'space-abc12345', title: 'research-tnxp-2026' },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    )
  })

  it('focuses a space through route state when a space is selected', async () => {
    const user = userEvent.setup()
    const setFocusedSpaceIdMock = vi.fn()
    mockUseNavigation.mockReturnValue({
      activeView: 'dashboard',
      setActiveView: vi.fn(),
      focusedProjectRoot: '/tmp/playground',
      focusedSpaceId: null,
      setFocusedSpaceId: setFocusedSpaceIdMock,
      setFocusedProjectRoot: vi.fn(),
    })
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-abc12345', title: 'Finance', status: 'open' }], isLoading: false })

    renderSidebar()

    await user.click(screen.getByTestId('space-row-space-abc12345'))

    expect(setFocusedSpaceIdMock).toHaveBeenCalledWith('space-abc12345')
  })

  it('renders member rows after explicit expansion', async () => {
    const user = userEvent.setup()
    mockUseNavigation.mockReturnValue({
      activeView: 'space',
      setActiveView: vi.fn(),
      focusedProjectRoot: '/tmp/playground',
      projectId: 'playground',
      focusedSpaceId: 'space-abc12345',
      setFocusedSpaceId: vi.fn(),
      setFocusedProjectRoot: vi.fn(),
    })
    mockUseSpaceList.mockReturnValue({ data: [{ id: 'space-abc12345', title: 'Finance', status: 'open' }], isLoading: false })
    mockUseSpaceMemberList.mockImplementation((params: { spaceId?: string; projectId?: string }) => {
      if (params?.spaceId === 'space-abc12345') {
        return {
          data: [
            {
              id: 'member-1',
              spaceId: 'space-abc12345',
              channelId: 'ch-member-1',
              displayName: 'researcher',
              memberType: 'worker',
              lifecycleState: 'active',
              harnessKind: 'codex',
              model: 'gpt-5.5',
              effort: 'medium',
            },
          ],
          isLoading: false,
          isError: false,
          refetch: vi.fn(),
        }
      }
      return { data: [], isLoading: false, isError: false, refetch: vi.fn() }
    })

    renderSidebar()

    expect(screen.getByTestId('space-row-space-abc12345')).toBeInTheDocument()
    await user.click(screen.getByTestId('space-member-toggle-space-abc12345'))

    await waitFor(() => {
      expect(screen.getByTestId('member-row-member-1')).toBeInTheDocument()
    })
  })

  it('shows slug space titles but hides raw space identifiers', () => {
    mockUseSpaceList.mockReturnValue({
      data: [
        { id: 'space-abc12345', title: 'research-tnxp' },
      ],
      isLoading: false,
    })

    renderSidebar()

    expect(screen.getByText('research-tnxp')).toBeInTheDocument()
    expect(screen.queryByText('abc12345')).toBeNull()
  })

})
