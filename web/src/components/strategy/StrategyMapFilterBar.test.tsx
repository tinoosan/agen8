import { render, screen, fireEvent } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'

import { StrategyMapFilterBar } from './StrategyMapFilterBar'

/* The search button is the touch-reachable entry point to node search. The
 * mobile top-bar search is md:hidden, so on iPad/tablet (desktop layout, no
 * hardware keyboard) this on-canvas button is the only way to open it. These
 * tests pin that it's present and fires the callback. */
function renderBar(overrides: Partial<Parameters<typeof StrategyMapFilterBar>[0]> = {}) {
  const onOpenSearch = vi.fn()
  const result = render(
    <StrategyMapFilterBar
      activeFilter={null}
      onFilterChange={() => {}}
      matchCount={0}
      onOpenSearch={onOpenSearch}
      {...overrides}
    />,
  )
  return { ...result, onOpenSearch }
}

describe('StrategyMapFilterBar — search button', () => {
  it('renders a search button (touch entry point, always visible)', () => {
    renderBar()
    expect(screen.getByLabelText('Search nodes')).toBeInTheDocument()
  })

  it('renders as normal flow content, not an absolute canvas overlay', () => {
    const { container } = renderBar()
    expect(container.firstElementChild).not.toHaveClass('absolute')
    expect(container.firstElementChild).toHaveClass('overflow-x-auto')
  })

  it('invokes onOpenSearch when clicked', () => {
    const { onOpenSearch } = renderBar()
    fireEvent.click(screen.getByLabelText('Search nodes'))
    expect(onOpenSearch).toHaveBeenCalledTimes(1)
  })

  it('does not pass a filter preset through the search button', () => {
    // The search button must not double as a filter toggle.
    const onFilterChange = vi.fn()
    renderBar({ onFilterChange })
    fireEvent.click(screen.getByLabelText('Search nodes'))
    expect(onFilterChange).not.toHaveBeenCalled()
  })
})
