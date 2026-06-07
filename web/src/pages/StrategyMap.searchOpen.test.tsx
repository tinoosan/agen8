import { StrictMode } from 'react'
import { describe, it, expect, beforeEach, vi } from 'vitest'
import { render, screen } from '@testing-library/react'
import { Router } from 'wouter'
import { memoryLocation } from 'wouter/memory-location'
import { QueryClient, QueryClientProvider } from '@tanstack/react-query'
import { useStore } from '../lib/store'

// The context map is heavy (React Flow canvas, framer-motion panels). This
// test only exercises the search-open lifecycle, so the visual children and
// the graph data hook are stubbed. The real StrategyMapSearch is kept so the
// assertion reflects the actual modal opening.
const node = { id: 'n1', type: 'mission', position: { x: 0, y: 0 }, data: { label: 'M1' } }

vi.mock('../components/strategy/useStrategyGraph', () => ({
  useStrategyGraph: () => ({ nodes: [node], edges: [], isLoading: false }),
}))
vi.mock('../components/strategy/StrategyMapCanvas', () => ({
  StrategyMapCanvas: () => <div data-testid="canvas" />,
}))
vi.mock('../components/strategy/StrategyMapLegend', () => ({
  StrategyMapLegend: () => null,
}))
vi.mock('../components/strategy/StrategyMapZoomControls', () => ({
  StrategyMapZoomControls: () => null,
}))
vi.mock('../components/strategy/StrategyMapFilterBar', () => ({
  StrategyMapFilterBar: () => null,
}))
vi.mock('../components/strategy/StrategyMapHelp', () => ({
  StrategyMapHelp: () => null,
}))

vi.mock('../lib/routing', async () => {
  const actual = await vi.importActual<typeof import('../lib/routing')>('../lib/routing')
  return { ...actual, useNavigation: () => ({ focusedProjectRoot: '/repo' }) }
})

const { default: StrategyMap } = await import('./StrategyMap')

function renderMap() {
  const location = memoryLocation({ path: '/project/p1/strategy', record: true })
  const client = new QueryClient({ defaultOptions: { queries: { retry: false } } })
  return render(
    <StrictMode>
      <QueryClientProvider client={client}>
        <Router hook={location.hook}>
          <StrategyMap projectId="p1" />
        </Router>
      </QueryClientProvider>
    </StrictMode>,
  )
}

describe('StrategyMap search-open on arrival', () => {
  beforeEach(() => {
    useStore.getState().setStrategySearchOpen(false)
    try { localStorage.clear() } catch { /* noop */ }
  })

  // Regression: the dashboard / mobile-top-bar search entry points set
  // strategySearchOpen before navigating here. The page must honor that flag
  // on mount — including under StrictMode's dev mount→unmount→remount, which a
  // reset-on-unmount cleanup used to clobber, leaving the modal shut.
  it('opens the node search when the flag is already true on mount (StrictMode)', () => {
    useStore.getState().setStrategySearchOpen(true)

    renderMap()

    expect(useStore.getState().strategySearchOpen).toBe(true)
    expect(screen.getByPlaceholderText(/Search nodes/i)).toBeInTheDocument()
  })

  it('leaves the node search closed when the flag is false on mount', () => {
    renderMap()

    expect(useStore.getState().strategySearchOpen).toBe(false)
    expect(screen.queryByPlaceholderText(/Search nodes/i)).not.toBeInTheDocument()
  })
})
