import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen, waitFor } from '@testing-library/react'
import userEvent from '@testing-library/user-event'
import { QueryClientProvider } from '@tanstack/react-query'
import CommandPalette from './CommandPalette'
import { createTestQueryClient } from '../test/test-utils'

const mockSetPaletteOpen = vi.fn()
const mockNavigate = vi.fn()
const mockUseMissions = vi.fn()

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
      projectId: 'project-1',
    }),
  }
})

vi.mock('wouter', () => ({
  useLocation: () => ['/project/project-1', mockNavigate],
}))

vi.mock('../hooks/useMissions', () => ({
  useMissions: (...args: unknown[]) => mockUseMissions(...args),
}))

describe('CommandPalette', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    vi.stubGlobal('ResizeObserver', class {
      observe() {}
      unobserve() {}
      disconnect() {}
    })
    mockUseMissions.mockReturnValue({
      data: [
        { id: 'mission-1', title: 'Launch MCP slice', status: 'active' },
        { id: 'mission-2', title: 'Archived work', status: 'archived' },
      ],
    })
  })

  it('lists active missions and focuses the strategy map on select', async () => {
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

    await waitFor(() => {
      expect(mockSetPaletteOpen).toHaveBeenCalledWith(false)
    })
  })

  it('navigates to account setup from the palette', async () => {
    const user = userEvent.setup()
    const queryClient = createTestQueryClient()
    render(
      <QueryClientProvider client={queryClient}>
        <CommandPalette />
      </QueryClientProvider>,
    )

    await user.click(screen.getByText('Account setup'))
    expect(mockNavigate).toHaveBeenCalledWith('/account')
  })
})
