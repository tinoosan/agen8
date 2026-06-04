import { act } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

const mockUseSpaceManifest = vi.fn()
const mockUseSpaceStatus = vi.fn()
const mockUseSpaceRoster = vi.fn()
const mockUseProjectSpace = vi.fn()
const mockUseProjectSpaces = vi.fn()
const mockUseSpace = vi.fn()
const mockUseSpaceMemberList = vi.fn()
const mockUseSpaceMemberRemove = vi.fn()
const mockUseSpaceMemberRegister = vi.fn()
const mockUseHarnessConfigOptions = vi.fn()
const mockRpcCall = vi.fn()
const mockApplyDesiredTemplateAction = vi.fn()

vi.mock('../hooks/useSpaceStatus', () => ({
  useSpaceManifest: (...args: unknown[]) => mockUseSpaceManifest(...args),
  useSpaceStatus: (...args: unknown[]) => mockUseSpaceStatus(...args),
  useSpaceRoster: (...args: unknown[]) => mockUseSpaceRoster(...args),
}))

vi.mock('../hooks/useSpace', () => ({
  useSpace: (...args: unknown[]) => mockUseSpace(...args),
  useSpaceMemberList: (...args: unknown[]) => mockUseSpaceMemberList(...args),
  useSpaceMemberRemove: () => mockUseSpaceMemberRemove(),
  useSpaceMemberRegister: () => mockUseSpaceMemberRegister(),
}))

vi.mock('../hooks/useHarnessConfigOptions', () => ({
  useHarnessConfigOptions: (...args: unknown[]) => mockUseHarnessConfigOptions(...args),
}))

vi.mock('../hooks/useProjectSpace', () => ({
  useProjectSpace: (...args: unknown[]) => mockUseProjectSpace(...args),
}))

vi.mock('../hooks/useProjectSpaces', () => ({
  useProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
}))

vi.mock('../lib/rpc', () => ({
  rpcCall: (...args: unknown[]) => mockRpcCall(...args),
  getStoredSessionToken: () => 'test-session-token',
  onNotification: () => () => {},
}))

vi.mock('../hooks/useDesiredTemplateManifestActions', () => ({
  useDesiredTemplateManifestActions: () => ({
    applyDesiredTemplateAction: mockApplyDesiredTemplateAction,
    isPending: false,
  }),
}))

vi.mock('../lib/routing', () => ({
  useNavigation: () => ({
    focusedProjectRoot: '/repo',
    focusedSpaceId: 'space-1',
    activeView: 'dashboard' as const,
    projectId: 'myapp',
    projectLoading: false,
    setFocusedProjectRoot: vi.fn(),
    setFocusedSpaceId: vi.fn(),
    setActiveView: vi.fn(),
  }),
}))

vi.mock('../components/ContextPanel', () => {
  function MockContextPanel({ spaceId }: { spaceId: string }) {
    return <div>Context Panel {spaceId}</div>
  }
  return { default: MockContextPanel }
})

vi.mock('../components/PulseDot', () => ({
  default: ({ status }: { status: string }) => <div>Pulse {status}</div>,
}))

const { default: SpaceFocus } = await import('./SpaceFocus')

const TEST_SPACE_ID = 'space-1'

function resetStore() {
  act(() => {
    window.history.replaceState({}, '', `/project/myapp/space/${TEST_SPACE_ID}`)
  })
}

function renderSpaceFocus(tab?: string) {
  if (tab) {
    window.history.pushState({}, '', `/project/p/space/${TEST_SPACE_ID}?tab=${tab}`)
  } else {
    window.history.pushState({}, '', `/project/p/space/${TEST_SPACE_ID}`)
  }
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false } },
  })
  const invalidateSpy = vi.spyOn(queryClient, 'invalidateQueries')

  render(
    <QueryClientProvider client={queryClient}>
      <SpaceFocus spaceId={TEST_SPACE_ID} />
    </QueryClientProvider>,
  )

  return { queryClient, invalidateSpy }
}

describe('SpaceFocus', () => {
  beforeEach(() => {
    resetStore()
    vi.clearAllMocks()
    // Clear ContextPanel view preference between tests so a prior
    // test's "plan" selection doesn't leak into the next test's
    // default render.
    try { localStorage.clear() } catch { /* ignore */ }

    mockUseSpaceManifest.mockReturnValue({
      data: {
        spaceName: 'Alpha Space',
        spaceId: 'space-1',
        coordinatorRole: 'coordinator',
        coordinatorRunId: 'run-1',
        spaceModel: 'gpt-5',
        roles: [],
      },
      isError: false,
    })

    mockUseSpaceStatus.mockReturnValue({
      data: {
        active: 1,
        pending: 0,
        done: 0,
      },
    })

    mockUseSpaceRoster.mockReturnValue({
      data: { spaceId: 'space-1', roles: [] },
      isLoading: false,
    })

    mockUseProjectSpace.mockReturnValue({
      data: {
        projectId: 'project-1',
        spaceId: 'space-1',
        spaceName: 'Alpha Space',
        coordinatorRunId: 'run-1',
        managedBy: 'desired-state',
        desiredEnabled: true,
      },
    })
    mockUseProjectSpaces.mockReturnValue({
      data: [{ spaceId: TEST_SPACE_ID, spaceName: 'Alpha Space' }],
    })
    // Default: space with no planMode set (most tests don't need it)
    mockUseSpace.mockReturnValue({
      data: { id: 'space-1', status: 'open' },
      isLoading: false,
    })
    mockUseSpaceMemberList.mockReturnValue({
      data: [],
      isLoading: false,
    })
    mockUseSpaceMemberRemove.mockReturnValue({ mutate: vi.fn(), isPending: false })
    mockUseSpaceMemberRegister.mockReturnValue({ mutate: vi.fn(), isPending: false })
    mockUseHarnessConfigOptions.mockReturnValue({
      data: {
        harnesses: [
          { kind: 'codex', models: [{ id: 'gpt-5.5', name: 'gpt-5.5', efforts: ['low', 'medium', 'high'] }] },
          { kind: 'claude-cli', models: [{ id: 'claude-opus-4-7', name: 'claude-opus-4-7', efforts: ['low', 'medium', 'high', 'xhigh', 'max'] }] },
        ],
      },
      isLoading: false,
    })
    mockRpcCall.mockImplementation(async (method: string) => {
      switch (method) {
        case 'space.list':
          return { threads: [{ id: 'space-1', title: 'Coordinator', status: 'open' }] }
        case 'space.get':
          return { space: { id: 'space-1', title: 'Coordinator', status: 'open' } }
        default:
          return {}
      }
    })
    vi.stubGlobal('fetch', vi.fn(async () => ({
      ok: true,
      json: async () => ({
        projectRoot: '/repo',
        claudeCommand: '/bin/claude',
        args: ['--remote-control'],
        commandLine: '/bin/claude --remote-control',
        pid: 1234,
        logPath: '/repo/.agen8/claude-launches/launch.log',
        remoteControlTitle: 'Agen8: space-1',
        channelRef: 'server:agen8-channel',
        developmentChannel: true,
        allowDangerouslySkipPermissions: true,
      }),
    })))
  })

  it('does not expose clear history from the removed More menu', () => {
    renderSpaceFocus()
    expect(screen.queryByRole('menuitem', { name: /clear space history/i })).not.toBeInTheDocument()
  })

  it('ignores removed tab names instead of redirecting them', async () => {
    renderSpaceFocus('plan')

    expect(screen.getByRole('tab', { name: /overview/i })).toHaveAttribute('data-state', 'active')
    expect(localStorage.getItem('oa-context-view:space-1')).toBeNull()
  })

  it('does not render the removed Inspector tab', () => {
    renderSpaceFocus()

    expect(screen.queryByRole('tab', { name: /inspector/i })).not.toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /overview/i })).toBeInTheDocument()
    expect(screen.getByRole('tab', { name: /board/i })).toBeInTheDocument()
  })

  it('falls back to overview for a stale ?tab=inspector deep link', () => {
    renderSpaceFocus('inspector')

    expect(screen.getByRole('tab', { name: /overview/i })).toHaveAttribute('data-state', 'active')
  })

  it('keeps the context panel scoped to the current space', async () => {
    mockUseSpaceManifest.mockReturnValue({
      data: {
        spaceName: 'Finance',
        spaceId: TEST_SPACE_ID,
        coordinatorRole: 'coordinator',
        coordinatorRunId: 'run-1',
        roles: [],
      },
      isError: false,
    })
    mockUseSpaceRoster.mockReturnValue({
      data: { spaceId: '', roles: [] },
      isLoading: false,
    })
    mockUseProjectSpace.mockReturnValue({
      data: {
        projectId: 'project-1',
        spaceId: TEST_SPACE_ID,
        spaceName: 'Finance',
        coordinatorRunId: 'run-1',
      },
    })
    mockUseProjectSpaces.mockReturnValue({
      data: [{ spaceId: TEST_SPACE_ID, spaceName: 'Finance' }],
    })

    renderSpaceFocus()
    expect(screen.getByText(`Context Panel ${TEST_SPACE_ID}`)).toBeInTheDocument()
  })

  it('resolves the context panel space from project records via spaceId', async () => {
    mockUseSpaceManifest.mockReturnValue({
      data: {
        spaceName: 'Finance Space',
        spaceId: TEST_SPACE_ID,
        coordinatorRole: 'coordinator',
        coordinatorRunId: 'run-1',
        roles: [],
      },
      isError: false,
    })
    mockUseProjectSpace.mockReturnValue({
      data: {
        projectId: 'project-1',
        spaceId: TEST_SPACE_ID,
        spaceName: 'Finance Space',
        coordinatorRunId: 'run-1',
      },
    })
    mockUseSpaceRoster.mockReturnValue({
      data: { spaceId: '', roles: [] },
      isLoading: false,
    })
    mockUseProjectSpaces.mockReturnValue({
      data: [
        { spaceId: TEST_SPACE_ID, spaceName: 'Finance Space', projectId: 'myapp' },
      ],
    })

    renderSpaceFocus()
    expect(screen.getByText(`Context Panel ${TEST_SPACE_ID}`)).toBeInTheDocument()
  })


  it('keeps files in the context rail and shows the top-right context toggle', () => {
    renderSpaceFocus()

    expect(screen.queryByRole('tab', { name: 'Files' })).not.toBeInTheDocument()
    expect(screen.getByTestId('context-panel-toggle')).toBeInTheDocument()
  })

  it('keeps the context rail available for a space with no runtime space bridge', async () => {
    const user = userEvent.setup()
    mockUseSpace.mockReturnValue({
      data: { id: TEST_SPACE_ID, title: 'Ad hoc space', status: 'open' },
      isLoading: false,
    })
    mockUseProjectSpace.mockReturnValue({ data: null })
    mockUseProjectSpaces.mockReturnValue({ data: [] })
    mockUseSpaceRoster.mockReturnValue({ data: { spaceId: '', roles: [] }, isLoading: false })
    mockUseSpaceManifest.mockReturnValue({ data: null, isError: false })

    renderSpaceFocus()
    await user.click(screen.getByTestId('context-panel-toggle'))

    expect(screen.getByText(`Context Panel ${TEST_SPACE_ID}`)).toBeInTheDocument()
  })

  it('does not expose the removed More actions menu', () => {
    renderSpaceFocus()
    expect(screen.queryByRole('button', { name: 'More actions' })).not.toBeInTheDocument()
  })

  it('does not expose desired-state disable from the removed More menu', () => {
    renderSpaceFocus()
    expect(screen.queryByRole('menuitem', { name: /disable space in manifest/i })).not.toBeInTheDocument()
  })

  it('does not show manifest actions for unmanaged spaces', () => {
    mockUseProjectSpace.mockReturnValue({
      data: {
        projectId: 'project-1',
        spaceId: 'space-1',
        spaceName: 'Alpha Space',
        coordinatorRunId: 'run-1',
        managedBy: 'runtime',
      },
    })

    renderSpaceFocus()

    expect(screen.queryByRole('menuitem', { name: /disable space in manifest/i })).not.toBeInTheDocument()
    expect(screen.queryByRole('menuitem', { name: /delete space from manifest/i })).not.toBeInTheDocument()
  })

  it('does not show legacy mode badges when space has no legacy mode', () => {
    // Default mock: space without legacy planMode set
    mockUseSpace.mockReturnValue({
      data: { id: 'space-1', status: 'open' },
      isLoading: false,
    })
    renderSpaceFocus()
    expect(screen.queryByTestId('space-plan-mode-badge')).not.toBeInTheDocument()
  })

  it('does not render legacy supervised badges in the space header', async () => {
    mockUseSpace.mockReturnValue({
      data: { id: 'space-1', status: 'open', planMode: 'supervised' },
      isLoading: false,
    })
    renderSpaceFocus()
    await waitFor(() => {
      expect(screen.queryByTestId('space-plan-mode-badge')).not.toBeInTheDocument()
    })
  })

  it('keeps legacy autonomous badges out of the space header', async () => {
    mockUseSpace.mockReturnValue({
      data: { id: 'space-1', status: 'open', planMode: 'autonomous' },
      isLoading: false,
    })
    renderSpaceFocus()
    await waitFor(() => {
      expect(screen.queryByTestId('space-plan-mode-badge')).not.toBeInTheDocument()
    })
  })

  it('launches Claude desktop through the daemon with the local bypass preset', async () => {
    const user = userEvent.setup()
    renderSpaceFocus()

    await user.click(screen.getByRole('button', { name: /launch claude/i }))
    expect(screen.getByText('Launch Claude desktop')).toBeInTheDocument()

    await user.click(screen.getByRole('button', { name: /start claude/i }))

    await waitFor(() => {
      expect(fetch).toHaveBeenCalledWith('/harness/claude/launch', expect.objectContaining({
        method: 'POST',
        headers: expect.objectContaining({
          Authorization: 'Bearer test-session-token',
        }),
        body: JSON.stringify({
          projectRoot: '/repo',
          remoteControlTitle: 'Agen8: space-1',
          channelRef: 'server:agen8-channel',
          developmentChannel: true,
          allowDangerouslySkipPermissions: true,
        }),
      }))
    })
    expect(await screen.findByText(/Claude launch started/i)).toBeInTheDocument()
    expect(screen.getByText(/PID 1234/i)).toBeInTheDocument()
  })

})
