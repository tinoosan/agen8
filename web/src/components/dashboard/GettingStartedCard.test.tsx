import { describe, it, expect, vi, beforeEach } from 'vitest'
import { render, screen, fireEvent, waitFor } from '@testing-library/react'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'

/* The card derives every tick from the three live project queries (the same
 * ones SSE invalidation refreshes), so the tests drive those mocks instead of
 * a network: changing what the members/missions hooks return and re-rendering
 * is exactly what a query invalidation does to the mounted card. */
const mockMembers = vi.fn()
const mockMissions = vi.fn()
const mockTasks = vi.fn()
const mockCreateAPIKey = vi.fn()

vi.mock('../../hooks/useProjectMembers', () => ({
  useProjectMembers: () => mockMembers(),
}))
vi.mock('../../hooks/useMissions', () => ({
  useMissions: () => mockMissions(),
}))
vi.mock('../../hooks/useProjectTasks', () => ({
  useProjectTasks: () => mockTasks(),
}))
vi.mock('../../lib/authClient', () => ({
  createAPIKey: (name: string) => mockCreateAPIKey(name),
}))

const { default: GettingStartedCard } = await import('./GettingStartedCard')

const ok = (count: number) => ({ isSuccess: true, data: Array.from({ length: count }, (_, i) => ({ id: `x${i}` })) })
const loading = { isSuccess: false, data: undefined }

function renderCard(projectId: string | null = 'p1') {
  const queryClient = new QueryClient({
    defaultOptions: { queries: { retry: false, refetchOnWindowFocus: false } },
  })
  return render(
    <QueryClientProvider client={queryClient}>
      <GettingStartedCard projectId={projectId} />
    </QueryClientProvider>,
  )
}

describe('GettingStartedCard', () => {
  beforeEach(() => {
    vi.clearAllMocks()
    localStorage.clear()
    mockMembers.mockReturnValue(ok(0))
    mockMissions.mockReturnValue(ok(0))
    mockTasks.mockReturnValue(ok(0))
  })

  it('renders nothing without a project or while queries load', () => {
    expect(renderCard(null).container).toBeEmptyDOMElement()
    mockMembers.mockReturnValue(loading)
    expect(renderCard().container).toBeEmptyDOMElement()
  })

  it('fresh project: shows the checklist with one Claude setup path and a token button', () => {
    renderCard()
    expect(screen.getByText('Getting started')).toBeInTheDocument()
    expect(screen.getByText('Project created')).toBeInTheDocument()
    expect(screen.getByText('Included in the Claude setup command above.')).toBeInTheDocument()
    expect(screen.getByRole('button', { name: 'Generate connect command' })).toBeInTheDocument()
  })

  it('first member registration ticks connect, skill, and agent without remount', () => {
    const { rerender, container } = renderCard()
    expect(container.textContent).toContain('Start an agent in this project folder — to do')
    mockMembers.mockReturnValue(ok(1))
    rerender(
      <QueryClientProvider client={new QueryClient()}>
        <GettingStartedCard projectId="p1" />
      </QueryClientProvider>,
    )
    expect(container.textContent).toContain('Start an agent in this project folder — done')
    expect(container.textContent).toContain('Connect your harness — done')
    expect(container.textContent).toContain('Give it work — to do')
  })

  it('hides itself once a member and work both exist', () => {
    mockMembers.mockReturnValue(ok(1))
    mockMissions.mockReturnValue(ok(1))
    expect(renderCard().container).toBeEmptyDOMElement()
  })

  it('dismiss hides the card and persists per project', () => {
    const { container } = renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Dismiss getting started checklist' }))
    expect(container).toBeEmptyDOMElement()
    expect(renderCard('p1').container).toBeEmptyDOMElement()
    expect(renderCard('p2').container).not.toBeEmptyDOMElement()
  })

  it('generates a token and shows the connect command with it inlined', async () => {
    mockCreateAPIKey.mockResolvedValue({ secret: 'ak_test_secret' })
    renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Generate connect command' }))
    await waitFor(() => {
      expect(screen.getByText(/agen8 client setup --harness claude/)).toBeInTheDocument()
    })
    expect(screen.getByText(/ak_test_secret/)).toBeInTheDocument()
    expect(mockCreateAPIKey).toHaveBeenCalledWith('Agen8 MCP key')
  })

  it('harness toggle switches the skill command to codex', () => {
    renderCard()
    fireEvent.click(screen.getByRole('button', { name: 'Codex' }))
    expect(screen.getByText('agen8 skill install --harness codex')).toBeInTheDocument()
  })
})
