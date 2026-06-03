import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import CommandPalette from './CommandPalette'
import { createTestQueryClient } from '../test/test-utils'

const mockSetPaletteOpen = vi.fn()
const mockSetFocusedSpaceId = vi.fn()
const mockNavigate = vi.fn()
const mockUseProjectSpaces = vi.fn()
const mockUseMissions = vi.fn()
const mockUsePendingOpActions = vi.fn()
const mockUsePendingEscalations = vi.fn()

vi.mock('../hooks/useFocusTrap', () => ({
  useFocusTrap: () => ({ current: null }),
}))

vi.mock('../lib/store', () => ({
  useStore: () => ({
    setPaletteOpen: mockSetPaletteOpen,
  }),
}))

vi.mock('../lib/routing', async () => {
  const actual = await vi.importActual<typeof import('../lib/routing')>('../lib/routing')
  return {
    ...actual,
    useNavigation: () => ({
      setFocusedSpaceId: mockSetFocusedSpaceId,
      focusedSpaceId: 'space-1',
      focusedProjectRoot: '/repo',
      projectId: 'project-1',
    }),
  }
})

vi.mock('wouter', () => ({
  useLocation: () => ['/project/project-1', mockNavigate],
}))

vi.mock('../hooks/useProjectSpaces', () => ({
  useProjectSpaces: (...args: unknown[]) => mockUseProjectSpaces(...args),
}))

vi.mock('../hooks/useMissions', () => ({
  useMissions: (...args: unknown[]) => mockUseMissions(...args),
}))

vi.mock('../hooks/useOpActions', () => ({
  usePendingOpActions: (...args: unknown[]) => mockUsePendingOpActions(...args),
}))

vi.mock('../hooks/useEscalations', () => ({
  usePendingEscalations: (...args: unknown[]) => mockUsePendingEscalations(...args),
}))

describe('CommandPalette', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
    mockUseProjectSpaces.mockReturnValue({
      data: [
        { spaceId: 'space-1', spaceName: 'alpha', status: 'idle' },
        { spaceId: 'space-2', spaceName: 'scratch', status: 'running' },
      ],
    })
    mockUseMissions.mockReturnValue({
      data: [
        { id: 'mission-1', title: 'Launch MCP slice', status: 'active' },
        { id: 'mission-2', title: 'Archived work', status: 'archived' },
      ],
    })
    mockUsePendingOpActions.mockReturnValue({
      data: [{ id: 'oa-1', title: 'Approve request', urgency: 'high', status: 'open' }],
    })
    mockUsePendingEscalations.mockReturnValue({
      data: [{ id: 'esc-1', title: 'Fix blocker', urgency: 'critical', category: 'build' }],
    })
  })

  it('focuses a selected space and closes the palette', async () => {
    const user = userEvent.setup()
    const queryClient = createTestQueryClient()
    render(
      <QueryClientProvider client={queryClient}>
        <CommandPalette />
      </QueryClientProvider>,
    )

    await user.click(screen.getByText('Alpha'))

    expect(mockSetFocusedSpaceId).toHaveBeenCalledWith('space-1')
    expect(mockSetPaletteOpen).toHaveBeenCalledWith(false)
  })

  it('navigates to missions, requests, escalations, and account setup', async () => {
    const user = userEvent.setup()
    const queryClient = createTestQueryClient()
    render(
      <QueryClientProvider client={queryClient}>
        <CommandPalette />
      </QueryClientProvider>,
    )

    expect(screen.queryByText('Archived work')).not.toBeInTheDocument()

    await user.click(screen.getByText('Launch MCP slice'))
    expect(mockNavigate).toHaveBeenCalledWith('/project/project-1/strategy?focus=mission-1')

    await user.click(screen.getByText('Approve request'))
    expect(mockNavigate).toHaveBeenCalledWith('/project/project-1/actions/oa-1')

    await user.click(screen.getByText('Fix blocker'))
    expect(mockNavigate).toHaveBeenCalledWith('/project/project-1/dashboard?panel=actions&type=escalation')

    await user.click(screen.getByText('Account setup'))
    expect(mockNavigate).toHaveBeenCalledWith('/account')

    await waitFor(() => {
      expect(mockSetPaletteOpen).toHaveBeenCalledWith(false)
    })
  })
})
