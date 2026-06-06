import { render, screen, within } from '@testing-library/react'
import { describe, expect, it } from 'vitest'

import { StrategyMapLegend, strategyMapLegendItems } from './StrategyMapLegend'

describe('StrategyMapLegend', () => {
  it('renders all Strategy Map node types', () => {
    render(<StrategyMapLegend />)

    const legend = screen.getByLabelText('Map Legend')
    for (const item of strategyMapLegendItems) {
      expect(within(legend).getByText(item)).toBeInTheDocument()
    }
  })

  it('uses quiet translucent map styling', () => {
    render(<StrategyMapLegend />)

    const legend = screen.getByLabelText('Map Legend')
    const style = legend.getAttribute('style') ?? ''

    expect(style).toContain('color-mix(in srgb, var(--bg-panel) 78%, transparent)')
    expect(style).toContain('color-mix(in srgb, var(--border) 52%, transparent)')
    expect(style).toContain('blur(10px)')
  })
})
